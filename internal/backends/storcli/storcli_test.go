package storcli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/system"
)

type fakeRunner struct {
	commands []system.Command
	result   system.Result
	err      error
}

func (f *fakeRunner) Run(_ context.Context, command system.Command) (system.Result, error) {
	f.commands = append(f.commands, command)
	return f.result, f.err
}

func TestParseMultiController(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "backends", "storcli.json"))
	if err != nil {
		t.Fatal(err)
	}
	slots, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(slots), 2; got != want {
		t.Fatalf("len(slots) = %d, want %d", got, want)
	}
	if got, want := slots[0].Bay.ID(), "c0/e41/s1"; got != want {
		t.Fatalf("slot[0] ID = %q, want %q", got, want)
	}
	if slots[0].Serial != "DISK-A-SERIAL" || slots[1].Bay.ID() != "c1/e90/s12" {
		t.Fatalf("slots = %#v", slots)
	}
}

func TestCommandForLocateAllowlist(t *testing.T) {
	t.Parallel()

	bay := app.Bay{Controller: "c0", Enclosure: "e41", Slot: "s2"}
	command, err := CommandForLocate("/opt/storcli64", bay, app.LEDActionOn)
	if err != nil {
		t.Fatal(err)
	}
	want := system.Command{Name: "/opt/storcli64", Args: []string{"/c0/e41/s2", "start", "locate"}}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command = %#v, want %#v", command, want)
	}

	bad := app.Bay{Controller: "c0", Enclosure: "e41", Slot: "s2 delete"}
	if _, err := CommandForLocate("/opt/storcli64", bad, app.LEDActionOn); err == nil {
		t.Fatal("unsafe target was accepted")
	}
	if _, err := CommandForLocate("/opt/storcli64", bay, app.LEDAction("erase")); err == nil {
		t.Fatal("unsafe action was accepted")
	}
}

func TestBackendLocateOnlyRunsAllowedCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{result: system.Result{Stdout: []byte("Status = Success\n")}}
	backend := Backend{Runner: runner, Executable: "/opt/storcli64"}
	bay := app.Bay{Backend: "storcli", Controller: "c1", Enclosure: "e2", Slot: "s3"}
	if err := backend.Locate(context.Background(), bay, app.LEDActionOff); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, []string{"/c1/e2/s3", "stop", "locate"}) {
		t.Fatalf("commands = %#v", runner.commands)
	}
}
