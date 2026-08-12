package storcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/system"
)

var (
	driveDetailPattern = regexp.MustCompile(`^Drive /c([0-9]+)/e([0-9]+)/s([0-9]+) - Detailed Information$`)
	targetPattern      = regexp.MustCompile(`^/c[0-9]+/e[0-9]+/s[0-9]+$`)
)

// Backend supports Broadcom StorCLI-compatible controllers.
type Backend struct {
	Runner     system.Runner
	Executable string
	Timeout    time.Duration
}

func (Backend) Name() string { return "storcli" }

func (b Backend) List(ctx context.Context, _ []app.Disk) ([]enclosure.Slot, error) {
	executable, err := b.executable()
	if err != nil {
		return nil, err
	}
	runner := b.Runner
	if runner == nil {
		runner = system.ExecRunner{}
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := runner.Run(commandCtx, system.Command{Name: executable, Args: []string{"/call/eall/sall", "show", "all", "J"}})
	if err != nil {
		return nil, err
	}
	return Parse(result.Stdout)
}

func (b Backend) Locate(ctx context.Context, bay app.Bay, action app.LEDAction) error {
	executable, err := b.executable()
	if err != nil {
		return err
	}
	command, err := CommandForLocate(executable, bay, action)
	if err != nil {
		return err
	}
	runner := b.Runner
	if runner == nil {
		runner = system.ExecRunner{}
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := runner.Run(commandCtx, command)
	if err != nil {
		return err
	}
	if !strings.Contains(string(result.Stdout), "Status = Success") && !strings.Contains(string(result.Stdout), `"Status" : "Success"`) {
		return &app.Error{Code: app.ErrorInternal, Op: "storcli locate", Message: "controller did not report success"}
	}
	return nil
}

func (b Backend) executable() (string, error) {
	if b.Executable != "" {
		return b.Executable, nil
	}
	for _, candidate := range []string{"storcli64", "storcli", "/opt/MegaRAID/storcli/storcli64"} {
		if strings.Contains(candidate, "/") {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", &app.Error{Code: app.ErrorUnsupported, Op: "storcli", Message: "StorCLI executable not found"}
}

// CommandForLocate is the sole StorCLI mutation constructor.
func CommandForLocate(executable string, bay app.Bay, action app.LEDAction) (system.Command, error) {
	target := fmt.Sprintf("/%s/%s/%s", bay.Controller, bay.Enclosure, bay.Slot)
	if executable == "" || !targetPattern.MatchString(target) {
		return system.Command{}, &app.Error{Code: app.ErrorMalformed, Op: "storcli locate", Message: "invalid controller bay target"}
	}
	verb := ""
	switch action {
	case app.LEDActionOn:
		verb = "start"
	case app.LEDActionOff:
		verb = "stop"
	default:
		return system.Command{}, &app.Error{Code: app.ErrorUnsupported, Op: "storcli locate", Message: "unsupported LED action"}
	}
	return system.Command{Name: executable, Args: []string{target, verb, "locate"}}, nil
}

type response struct {
	Controllers []controller `json:"Controllers"`
}

type controller struct {
	CommandStatus map[string]any             `json:"Command Status"`
	ResponseData  map[string]json.RawMessage `json:"Response Data"`
}

// Parse converts StorCLI JSON into backend-neutral slots.
func Parse(data []byte) ([]enclosure.Slot, error) {
	var output response
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse storcli", Message: "invalid JSON", Err: err}
	}
	var slots []enclosure.Slot
	for _, controllerResponse := range output.Controllers {
		if status := strings.TrimSpace(stringValue(controllerResponse.CommandStatus["Status"])); status != "" && status != "Success" {
			return nil, &app.Error{Code: app.ErrorInternal, Op: "parse storcli", Message: "controller query failed"}
		}
		for key, raw := range controllerResponse.ResponseData {
			matches := driveDetailPattern.FindStringSubmatch(key)
			if len(matches) != 4 {
				continue
			}
			attributes, err := deviceAttributes(raw)
			if err != nil {
				return nil, err
			}
			slots = append(slots, enclosure.Slot{
				Bay: app.Bay{
					Backend:    "storcli",
					Controller: "c" + matches[1],
					Enclosure:  "e" + matches[2],
					Slot:       "s" + matches[3],
					Capability: app.LEDCapabilityLocate,
					LEDState:   app.LEDStateUnknown,
				},
				Serial: stringValue(attributes["SN"]),
				WWN:    stringValue(attributes["WWN"]),
				Model:  stringValue(attributes["Model Number"]),
			})
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Bay.ID() < slots[j].Bay.ID() })
	return slots, nil
}

func deviceAttributes(raw json.RawMessage) (map[string]any, error) {
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse storcli", Message: "invalid drive details", Err: err}
	}
	for key, value := range detail {
		if !strings.HasSuffix(key, " Device attributes") {
			continue
		}
		var attributes map[string]any
		if err := json.Unmarshal(value, &attributes); err != nil {
			return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse storcli", Message: "invalid drive attributes", Err: err}
		}
		return attributes, nil
	}
	return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse storcli", Message: "drive attributes are missing"}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}
