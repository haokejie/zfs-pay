package ledctl

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/system"
)

var safeDevice = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Backend is disabled by default because ledctl support varies by controller
// and some versions may affect LEDs not named in the request.
type Backend struct {
	Runner     system.Runner
	Executable string
	Timeout    time.Duration
	Enabled    bool
}

func (Backend) Name() string { return "ledctl" }

func (b Backend) List(_ context.Context, disks []app.Disk) ([]enclosure.Slot, error) {
	if !b.Enabled {
		return nil, &app.Error{Code: app.ErrorUnsupported, Op: "ledctl", Message: "backend is disabled by default"}
	}
	executable, err := b.executable()
	if err != nil {
		return nil, err
	}
	_ = executable
	var slots []enclosure.Slot
	for _, disk := range disks {
		device := filepath.Base(disk.ParentDevice)
		if !safeDevice.MatchString(device) {
			continue
		}
		slots = append(slots, enclosure.Slot{
			Bay: app.Bay{
				Backend:    "ledctl",
				Controller: "linux",
				Enclosure:  "block",
				Slot:       device,
				Capability: app.LEDCapabilityLocate,
				LEDState:   app.LEDStateUnknown,
			},
			Serial: disk.Serial,
			WWN:    disk.WWN,
			Model:  disk.Model,
			Device: disk.ParentDevice,
		})
	}
	return slots, nil
}

func (b Backend) Locate(ctx context.Context, bay app.Bay, action app.LEDAction) error {
	if !b.Enabled {
		return &app.Error{Code: app.ErrorUnsupported, Op: "ledctl", Message: "backend is disabled by default"}
	}
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
	_, err = runner.Run(commandCtx, command)
	return err
}

func CommandForLocate(executable string, bay app.Bay, action app.LEDAction) (system.Command, error) {
	if executable == "" || bay.Controller != "linux" || bay.Enclosure != "block" || !isSafeDevice(bay.Slot) {
		return system.Command{}, &app.Error{Code: app.ErrorMalformed, Op: "ledctl locate", Message: "invalid block device target"}
	}
	pattern := ""
	switch action {
	case app.LEDActionOn:
		pattern = "locate="
	case app.LEDActionOff:
		pattern = "locate_off="
	default:
		return system.Command{}, &app.Error{Code: app.ErrorUnsupported, Op: "ledctl locate", Message: "unsupported LED action"}
	}
	return system.Command{Name: executable, Args: []string{pattern + filepath.Join("/dev", bay.Slot)}}, nil
}

func isSafeDevice(value string) bool {
	return value != "." && value != ".." && safeDevice.MatchString(value) && filepath.Base(value) == value
}

func (b Backend) executable() (string, error) {
	if b.Executable != "" {
		return b.Executable, nil
	}
	path, err := exec.LookPath("ledctl")
	if err != nil {
		return "", &app.Error{Code: app.ErrorUnsupported, Op: "ledctl", Message: "ledctl executable not found", Err: err}
	}
	return path, nil
}
