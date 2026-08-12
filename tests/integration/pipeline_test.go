package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/automation"
	"github.com/zoujunkun/zfs-pay/internal/backends/storcli"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
	"github.com/zoujunkun/zfs-pay/internal/state"
	"github.com/zoujunkun/zfs-pay/internal/system"
	"github.com/zoujunkun/zfs-pay/internal/zfs"
)

type fixtureRunner struct {
	mu       sync.Mutex
	status   []byte
	lsblk    []byte
	storcli  []byte
	commands []system.Command
	failZFS  bool
}

func (r *fixtureRunner) Run(_ context.Context, command system.Command) (system.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
	switch command.Name {
	case "zpool":
		if r.failZFS {
			return system.Result{}, &app.Error{Code: app.ErrorUnsupported, Op: "zpool", Message: "command not found"}
		}
		return system.Result{Stdout: append([]byte(nil), r.status...)}, nil
	case "lsblk":
		return system.Result{Stdout: append([]byte(nil), r.lsblk...)}, nil
	case "storcli64":
		if reflect.DeepEqual(command.Args, []string{"/call/eall/sall", "show", "all", "J"}) {
			return system.Result{Stdout: append([]byte(nil), r.storcli...)}, nil
		}
		return system.Result{Stdout: []byte("Status = Success\n")}, nil
	default:
		return system.Result{}, &app.Error{Code: app.ErrorUnsupported, Op: command.Name, Message: "command not found"}
	}
}

func (r *fixtureRunner) setStatus(status []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *fixtureRunner) locateCommands() []system.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	var commands []system.Command
	for _, command := range r.commands {
		if command.Name == "storcli64" && len(command.Args) == 3 {
			commands = append(commands, command)
		}
	}
	return commands
}

func TestPipelineCachesMissingDiskAndOwnsLocateLED(t *testing.T) {
	online := fixture(t, "zpool-online.txt")
	runner := &fixtureRunner{
		status:  online,
		lsblk:   fixture(t, "lsblk.json"),
		storcli: fixture(t, "storcli.json"),
	}
	discoverer := &inventory.Discoverer{
		ZFS: zfs.Provider{Runner: runner},
		Identity: system.BlockProvider{Runner: runner, ResolvePath: func(path string) (string, error) {
			resolved := map[string]string{
				"/dev/disk/by-id/wwn-test-a-part1": "/dev/sda1",
				"/dev/disk/by-id/wwn-test-b-part1": "/dev/sdb1",
			}
			return resolved[path], nil
		}},
	}
	enclosures := &enclosure.Service{Backends: []enclosure.Backend{
		storcli.Backend{Runner: runner, Executable: "storcli64"},
	}}
	engine := automation.Engine{
		Inventory:  discoverer,
		Enclosures: enclosures,
		Store:      state.Store{Path: filepath.Join(t.TempDir(), "state.json")},
	}

	result, err := engine.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 0 {
		t.Fatalf("healthy startup changed LEDs: %#v", result.Operations)
	}
	data, err := engine.Store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Mappings) != 2 || data.Mappings["7100000000000002"].Bay.ID() != "c1/e20/s9" {
		t.Fatalf("multi-controller mappings = %#v", data.Mappings)
	}

	runner.setStatus(fixture(t, "zpool-unavail.txt"))
	result, err = engine.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || !result.Operations[0].UsedCache || result.Operations[0].Action != app.LEDActionOn {
		t.Fatalf("missing disk did not use verified cache: %#v", result)
	}

	runner.setStatus(online)
	result, err = engine.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Action != app.LEDActionOff {
		t.Fatalf("recovered disk did not clear managed LED: %#v", result)
	}
	want := [][]string{{"/c1/e20/s9", "start", "locate"}, {"/c1/e20/s9", "stop", "locate"}}
	commands := runner.locateCommands()
	got := make([][]string, len(commands))
	for index := range commands {
		got[index] = commands[index].Args
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("controller mutations = %#v, want only %#v", got, want)
	}
}

func TestAmbiguousIdentityFailsClosed(t *testing.T) {
	runner := &fixtureRunner{storcli: fixture(t, "storcli-ambiguous.json")}
	service := &enclosure.Service{Backends: []enclosure.Backend{
		storcli.Backend{Runner: runner, Executable: "storcli64"},
	}}
	disks, _ := service.Map(context.Background(), []app.Disk{{VdevGUID: "1", Serial: "DUPLICATE"}})
	if disks[0].Bay != nil || len(disks[0].Diagnostics) != 1 || disks[0].Diagnostics[0].Code != app.ErrorAmbiguous {
		t.Fatalf("ambiguous mapping did not fail closed: %#v", disks[0])
	}
	if len(runner.locateCommands()) != 0 {
		t.Fatal("ambiguous discovery issued a controller mutation")
	}
}

func TestMissingZpoolIsUnsupported(t *testing.T) {
	runner := &fixtureRunner{failZFS: true}
	_, err := (zfs.Provider{Runner: runner}).Discover(context.Background())
	var typed *app.Error
	if !errors.As(err, &typed) || typed.Code != app.ErrorUnsupported {
		t.Fatalf("error = %v, want unsupported", err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "integration", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
