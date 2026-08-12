package system

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

// Identity is the stable Linux identity for one physical disk.
type Identity struct {
	Device       string
	ParentDevice string
	Serial       string
	WWN          string
	Model        string
}

// IdentityProvider resolves ZFS paths to physical disk identity in one batch.
type IdentityProvider interface {
	Resolve(context.Context, []string) (map[string]Identity, map[string][]app.Diagnostic, error)
}

// BlockProvider combines one lsblk snapshot with optional per-disk udev data.
type BlockProvider struct {
	Runner      Runner
	Timeout     time.Duration
	ResolvePath func(string) (string, error)
}

type lsblkOutput struct {
	BlockDevices []blockNode `json:"blockdevices"`
}

type blockNode struct {
	Name     string      `json:"name"`
	KName    string      `json:"kname"`
	Path     string      `json:"path"`
	Type     string      `json:"type"`
	PKName   string      `json:"pkname"`
	Serial   string      `json:"serial"`
	WWN      string      `json:"wwn"`
	Model    string      `json:"model"`
	Children []blockNode `json:"children"`
}

func (p BlockProvider) Resolve(ctx context.Context, paths []string) (map[string]Identity, map[string][]app.Diagnostic, error) {
	runner := p.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	resolvePath := p.ResolvePath
	if resolvePath == nil {
		resolvePath = filepath.EvalSymlinks
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := runner.Run(commandCtx, Command{
		Name: "lsblk",
		Args: []string{"--json", "--paths", "--output", "NAME,KNAME,PATH,TYPE,PKNAME,SERIAL,WWN,MODEL"},
	})
	if err != nil {
		return nil, nil, err
	}
	index, err := parseLSBLK(result.Stdout)
	if err != nil {
		return nil, nil, err
	}

	identities := make(map[string]Identity, len(paths))
	diagnostics := make(map[string][]app.Diagnostic)
	for _, input := range paths {
		resolved := input
		if candidate, resolveErr := resolvePath(input); resolveErr == nil && candidate != "" {
			resolved = candidate
		}
		node, ok := index.find(resolved)
		if !ok {
			diagnostics[input] = append(diagnostics[input], app.Diagnostic{Code: app.ErrorNotFound, Source: "lsblk", Message: "block device not found"})
			continue
		}
		physical := index.physical(node)
		identity := Identity{
			Device:       cleanDevicePath(node.Path, node.Name, node.KName),
			ParentDevice: cleanDevicePath(physical.Path, physical.Name, physical.KName),
			Serial:       normalizeIdentity(physical.Serial),
			WWN:          normalizeWWN(physical.WWN),
			Model:        strings.TrimSpace(physical.Model),
		}
		if identity.Serial == "" || identity.WWN == "" || identity.Model == "" {
			properties, propertyErr := p.udevProperties(ctx, runner, timeout, identity.ParentDevice)
			if propertyErr != nil {
				diagnostics[input] = append(diagnostics[input], diagnosticFromError("udevadm", "unable to read complete udev identity", propertyErr))
			} else {
				if identity.Serial == "" {
					identity.Serial = firstNormalized(properties, "ID_SCSI_SERIAL", "ID_SERIAL_SHORT", "SCSI_IDENT_SERIAL", "ID_SERIAL")
				}
				if identity.WWN == "" {
					identity.WWN = normalizeWWN(firstValue(properties, "ID_WWN_WITH_EXTENSION", "ID_WWN"))
				}
				if identity.Model == "" {
					identity.Model = strings.TrimSpace(firstValue(properties, "ID_MODEL", "ID_SCSI_MODEL"))
				}
			}
		}
		identities[input] = identity
	}
	return identities, diagnostics, nil
}

func (p BlockProvider) udevProperties(ctx context.Context, runner Runner, timeout time.Duration, device string) (map[string]string, error) {
	if device == "" {
		return nil, &app.Error{Code: app.ErrorNotFound, Op: "udevadm", Message: "parent block device is unknown"}
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := runner.Run(commandCtx, Command{Name: "udevadm", Args: []string{"info", "--query=property", "--name", device}})
	if err != nil {
		return nil, err
	}
	return parseProperties(result.Stdout), nil
}

type blockIndex struct {
	byPath   map[string]*blockNode
	byName   map[string]*blockNode
	parentOf map[*blockNode]*blockNode
}

func parseLSBLK(data []byte) (*blockIndex, error) {
	var output lsblkOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, &app.Error{Code: app.ErrorMalformed, Op: "parse lsblk", Message: "invalid JSON", Err: err}
	}
	index := &blockIndex{byPath: make(map[string]*blockNode), byName: make(map[string]*blockNode), parentOf: make(map[*blockNode]*blockNode)}
	var add func(*blockNode, *blockNode)
	add = func(node, parent *blockNode) {
		if node.Path != "" {
			index.byPath[filepath.Clean(node.Path)] = node
		}
		for _, value := range []string{node.Name, node.KName} {
			if value == "" {
				continue
			}
			index.byName[filepath.Base(value)] = node
		}
		if parent != nil {
			index.parentOf[node] = parent
		}
		for i := range node.Children {
			add(&node.Children[i], node)
		}
	}
	for i := range output.BlockDevices {
		add(&output.BlockDevices[i], nil)
	}
	return index, nil
}

func (i *blockIndex) find(path string) (*blockNode, bool) {
	if node, ok := i.byPath[filepath.Clean(path)]; ok {
		return node, true
	}
	node, ok := i.byName[filepath.Base(path)]
	return node, ok
}

func (i *blockIndex) physical(node *blockNode) *blockNode {
	current := node
	for current != nil && current.Type == "part" {
		if parent := i.parentOf[current]; parent != nil {
			current = parent
			continue
		}
		if current.PKName != "" {
			if parent, ok := i.byName[filepath.Base(current.PKName)]; ok {
				current = parent
				continue
			}
		}
		break
	}
	return current
}

func cleanDevicePath(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "/") {
			return filepath.Clean(value)
		}
		return filepath.Join("/dev", filepath.Base(value))
	}
	return ""
}

func normalizeIdentity(value string) string {
	return strings.ToUpper(strings.Trim(strings.TrimSpace(value), "\x00"))
}

func normalizeWWN(value string) string {
	value = normalizeIdentity(value)
	return strings.TrimPrefix(value, "0X")
}

func parseProperties(data []byte) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key != "" {
			properties[key] = value
		}
	}
	return properties
}

func firstValue(properties map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := properties[key]; strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNormalized(properties map[string]string, keys ...string) string {
	return normalizeIdentity(firstValue(properties, keys...))
}

func diagnosticFromError(source, message string, err error) app.Diagnostic {
	code := app.ErrorInternal
	if typed, ok := err.(*app.Error); ok {
		code = typed.Code
	}
	return app.Diagnostic{Code: code, Source: source, Message: message}
}
