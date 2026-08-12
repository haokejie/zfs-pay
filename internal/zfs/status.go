package zfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/system"
)

// Leaf is one physical leaf vdev reported by OpenZFS.
type Leaf struct {
	Pool   string
	GUID   string
	Name   string
	Path   string
	Role   string
	State  app.Health
	Errors app.ErrorCounters
	Depth  int
}

// Provider discovers leaf vdevs using only read-only zpool status commands.
type Provider struct {
	Runner  system.Runner
	Timeout time.Duration
}

func (p Provider) Discover(ctx context.Context) ([]Leaf, error) {
	runner := p.Runner
	if runner == nil {
		runner = system.ExecRunner{}
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	preferred, preferredErr := runner.Run(commandCtx, system.Command{
		Name: "zpool",
		Args: []string{"status", "-g", "-p", "-c", "upath"},
		Env:  []string{"ZPOOL_SCRIPTS_AS_ROOT=1"},
	})
	if preferredErr == nil {
		leaves, err := ParseStatus(preferred.Stdout, true)
		if err == nil {
			return leaves, nil
		}
		preferredErr = err
	}
	if noPools(preferred.Stdout, preferred.Stderr) {
		return nil, nil
	}

	guidCtx, guidCancel := context.WithTimeout(ctx, timeout)
	defer guidCancel()
	guidResult, guidErr := runner.Run(guidCtx, system.Command{Name: "zpool", Args: []string{"status", "-g", "-p"}})
	if guidErr != nil {
		if noPools(guidResult.Stdout, guidResult.Stderr) {
			return nil, nil
		}
		return nil, wrapDiscoveryError("zpool status GUID fallback", preferredErr, guidErr)
	}

	pathCtx, pathCancel := context.WithTimeout(ctx, timeout)
	defer pathCancel()
	pathResult, pathErr := runner.Run(pathCtx, system.Command{Name: "zpool", Args: []string{"status", "-P", "-p"}})
	if pathErr != nil {
		return nil, wrapDiscoveryError("zpool status path fallback", preferredErr, pathErr)
	}

	guidLeaves, err := ParseStatus(guidResult.Stdout, false)
	if err != nil {
		return nil, err
	}
	pathLeaves, err := ParseStatus(pathResult.Stdout, false)
	if err != nil {
		return nil, err
	}
	return MergeSnapshots(guidLeaves, pathLeaves)
}

func noPools(stdout, stderr []byte) bool {
	text := strings.ToLower(string(append(append([]byte{}, stdout...), stderr...)))
	return strings.Contains(text, "no pools available")
}

func wrapDiscoveryError(op string, earlier, current error) error {
	if current == nil {
		current = earlier
	}
	var typed *app.Error
	if errors.As(current, &typed) {
		return &app.Error{Code: typed.Code, Op: op, Message: typed.Message, Err: current}
	}
	return &app.Error{Code: app.ErrorInternal, Op: op, Message: "unable to query ZFS topology", Err: current}
}

type row struct {
	Leaf
	isRoot bool
}

// ParseStatus parses zpool's documented script-friendly numeric output. When
// withUPath is true, the final column must be the bundled upath zpool script.
func ParseStatus(output []byte, withUPath bool) ([]Leaf, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var all []Leaf
	var rows []row
	pool := ""
	role := "data"
	inConfig := false
	headerSeen := false

	flush := func() {
		for i := range rows {
			if rows[i].isRoot {
				continue
			}
			if i+1 < len(rows) && rows[i+1].Depth > rows[i].Depth {
				continue
			}
			leaf := rows[i].Leaf
			if leaf.Path == "-" {
				leaf.Path = ""
			}
			if leaf.Name == "" {
				leaf.Name = filepath.Base(leaf.Path)
			}
			all = append(all, leaf)
		}
		rows = nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pool:") {
			flush()
			pool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			role = "data"
			inConfig = false
			headerSeen = false
			continue
		}
		if trimmed == "config:" {
			inConfig = true
			continue
		}
		if strings.HasPrefix(trimmed, "errors:") {
			flush()
			inConfig = false
			continue
		}
		if !inConfig || trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "NAME") && strings.Contains(trimmed, "STATE") {
			headerSeen = true
			continue
		}
		if !headerSeen {
			continue
		}
		if mapped, ok := roleHeading(trimmed); ok {
			role = mapped
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 5 || !isState(fields[1]) {
			continue
		}
		readErrors, readErr := strconv.ParseUint(fields[2], 10, 64)
		writeErrors, writeErr := strconv.ParseUint(fields[3], 10, 64)
		checksumErrors, checksumErr := strconv.ParseUint(fields[4], 10, 64)
		if readErr != nil || writeErr != nil || checksumErr != nil {
			return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse zpool status", Message: "invalid error counters"}
		}
		name := fields[0]
		path := ""
		guid := ""
		if withUPath {
			guid = name
			if len(fields) >= 6 {
				path = fields[len(fields)-1]
			}
		} else {
			path = name
		}
		rows = append(rows, row{Leaf: Leaf{
			Pool:  pool,
			GUID:  guid,
			Name:  name,
			Path:  path,
			Role:  role,
			State: app.Health(fields[1]),
			Errors: app.ErrorCounters{
				Read:     readErrors,
				Write:    writeErrors,
				Checksum: checksumErrors,
			},
			Depth: leadingWidth(line),
		}, isRoot: name == pool})
	}
	if err := scanner.Err(); err != nil {
		return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse zpool status", Message: "status output is too large or unreadable", Err: err}
	}
	flush()
	if pool == "" && len(bytesTrimSpace(output)) > 0 && !noPools(output, nil) {
		return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse zpool status", Message: "no pool blocks found"}
	}
	return all, nil
}

func bytesTrimSpace(value []byte) []byte { return []byte(strings.TrimSpace(string(value))) }

func roleHeading(value string) (string, bool) {
	switch value {
	case "logs":
		return "log", true
	case "cache":
		return "cache", true
	case "spares":
		return "spare", true
	case "special":
		return "special", true
	case "dedup":
		return "dedup", true
	default:
		return "", false
	}
}

func isState(value string) bool {
	switch value {
	case "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "REMOVED", "UNAVAIL", "AVAIL", "INUSE":
		return true
	default:
		return false
	}
}

func leadingWidth(value string) int {
	width := 0
	for _, r := range value {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 8
		default:
			return width
		}
	}
	return width
}

// MergeSnapshots strictly pairs separate GUID and path snapshots. A topology
// change between commands is rejected rather than guessed.
func MergeSnapshots(guidLeaves, pathLeaves []Leaf) ([]Leaf, error) {
	if len(guidLeaves) != len(pathLeaves) {
		return nil, &app.Error{Code: app.ErrorAmbiguous, Op: "merge zpool status", Message: "ZFS topology changed between fallback snapshots"}
	}
	merged := make([]Leaf, len(guidLeaves))
	for i := range guidLeaves {
		guidLeaf, pathLeaf := guidLeaves[i], pathLeaves[i]
		if guidLeaf.Pool != pathLeaf.Pool || guidLeaf.Role != pathLeaf.Role || guidLeaf.State != pathLeaf.State ||
			guidLeaf.Errors != pathLeaf.Errors || guidLeaf.Depth != pathLeaf.Depth {
			return nil, &app.Error{Code: app.ErrorAmbiguous, Op: "merge zpool status", Message: fmt.Sprintf("ZFS topology changed near disk %d", i+1)}
		}
		merged[i] = pathLeaf
		merged[i].GUID = guidLeaf.Name
		if merged[i].Path == "-" || !strings.HasPrefix(merged[i].Path, "/") {
			merged[i].Path = ""
		}
		if merged[i].Name == "" {
			merged[i].Name = guidLeaf.Name
		}
	}
	return merged, nil
}
