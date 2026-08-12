package ses

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
)

var safeComponent = regexp.MustCompile(`^[A-Za-z0-9:._ -]+$`)

// Backend controls Linux SES sysfs entries when the kernel exposes an exact
// block-device-to-enclosure mapping.
type Backend struct {
	Root string
}

func (Backend) Name() string { return "ses" }

func (b Backend) List(_ context.Context, disks []app.Disk) ([]enclosure.Slot, error) {
	root := b.root()
	var slots []enclosure.Slot
	for _, disk := range disks {
		device := filepath.Base(disk.ParentDevice)
		if device == "." || device == "" || device == "/" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, "class", "block", device, "device", "enclosure_device:*"))
		if err != nil {
			return nil, &app.Error{Code: app.ErrorMalformed, Op: "ses list", Message: "invalid sysfs glob", Err: err}
		}
		for _, match := range matches {
			target, err := filepath.EvalSymlinks(match)
			if err != nil {
				continue
			}
			slotValue, err := os.ReadFile(filepath.Join(target, "slot"))
			if err != nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(target, "locate")); err != nil {
				continue
			}
			slots = append(slots, enclosure.Slot{
				Bay: app.Bay{
					Backend:    "ses",
					Controller: "ses",
					Enclosure:  filepath.Base(filepath.Dir(target)),
					Slot:       strings.TrimSpace(string(slotValue)),
					Capability: app.LEDCapabilityLocate,
					LEDState:   readLEDState(filepath.Join(target, "locate")),
				},
				Serial: disk.Serial,
				WWN:    disk.WWN,
				Model:  disk.Model,
				Device: disk.ParentDevice,
			})
		}
	}
	return slots, nil
}

func (b Backend) Locate(_ context.Context, bay app.Bay, action app.LEDAction) error {
	if bay.Controller != "ses" || !isSafeComponent(bay.Enclosure) || !isSafeComponent(bay.Slot) {
		return &app.Error{Code: app.ErrorMalformed, Op: "ses locate", Message: "invalid enclosure target"}
	}
	entries, err := os.ReadDir(filepath.Join(b.root(), "class", "enclosure", bay.Enclosure))
	if err != nil {
		return &app.Error{Code: app.ErrorNotFound, Op: "ses locate", Message: "enclosure not found", Err: err}
	}
	for _, entry := range entries {
		target := filepath.Join(b.root(), "class", "enclosure", bay.Enclosure, entry.Name())
		slotValue, readErr := os.ReadFile(filepath.Join(target, "slot"))
		if readErr != nil || strings.TrimSpace(string(slotValue)) != bay.Slot {
			continue
		}
		value := ""
		switch action {
		case app.LEDActionOn:
			value = "1"
		case app.LEDActionOff:
			value = "0"
		default:
			return &app.Error{Code: app.ErrorUnsupported, Op: "ses locate", Message: "unsupported LED action"}
		}
		locatePath := filepath.Join(target, "locate")
		if err := os.WriteFile(locatePath, []byte(value), 0); err != nil {
			return &app.Error{Code: app.ErrorPermission, Op: "ses locate", Message: "unable to write locate LED", Err: err}
		}
		state := readLEDState(locatePath)
		if (action == app.LEDActionOn && state != app.LEDStateOn) || (action == app.LEDActionOff && state != app.LEDStateOff) {
			return &app.Error{Code: app.ErrorUnsupported, Op: "ses locate", Message: "enclosure did not retain locate LED state"}
		}
		return nil
	}
	return &app.Error{Code: app.ErrorNotFound, Op: "ses locate", Message: fmt.Sprintf("slot %s not found", bay.Slot)}
}

func isSafeComponent(value string) bool {
	return value != "." && value != ".." && safeComponent.MatchString(value)
}

func (b Backend) root() string {
	if b.Root != "" {
		return b.Root
	}
	return "/sys"
}

func readLEDState(path string) app.LEDState {
	value, err := os.ReadFile(path)
	if err != nil {
		return app.LEDStateUnknown
	}
	if strings.TrimSpace(string(value)) == "0" {
		return app.LEDStateOff
	}
	return app.LEDStateOn
}
