package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
)

type inventoryStub struct {
	snapshot inventory.Snapshot
	err      error
}

func (s inventoryStub) Discover(context.Context) (inventory.Snapshot, error) {
	return s.snapshot, s.err
}

type enclosureStub struct {
	disks       []app.Disk
	diagnostics []app.Diagnostic
	mu          sync.Mutex
	actions     []app.LEDAction
}

func (s *enclosureStub) Map(context.Context, []app.Disk) ([]app.Disk, []app.Diagnostic) {
	return append([]app.Disk(nil), s.disks...), s.diagnostics
}

func (s *enclosureStub) Plan(bay app.Bay, action app.LEDAction) (enclosure.LocatePlan, error) {
	return enclosure.LocatePlan{Backend: bay.Backend, Target: bay.ID(), Action: action}, nil
}

func (s *enclosureStub) Locate(_ context.Context, _ app.Bay, action app.LEDAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, action)
	return nil
}

func (s *enclosureStub) recordedActions() []app.LEDAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]app.LEDAction(nil), s.actions...)
}

func fixtureDisks() []app.Disk {
	return []app.Disk{
		{Pool: "tank", VdevGUID: "1", State: app.HealthOnline, ParentDevice: "/dev/sda", Serial: "SERIAL-A", WWN: "5000000000000001", Bay: &app.Bay{Backend: "storcli", Controller: "c0", Enclosure: "e41", Slot: "s1", Capability: app.LEDCapabilityLocate, LEDState: app.LEDStateOff}},
		{Pool: "tank", VdevGUID: "2", State: app.HealthDegraded, ParentDevice: "/dev/sdb", Serial: "SERIAL-B", Bay: &app.Bay{Backend: "storcli", Controller: "c0", Enclosure: "e41", Slot: "s2", Capability: app.LEDCapabilityLocate, LEDState: app.LEDStateUnknown}},
	}
}

func TestStatusTextMatchesGolden(t *testing.T) {
	t.Parallel()

	backend := &enclosureStub{disks: fixtureDisks()}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"status"}); got != 0 {
		t.Fatalf("Run(status) = %d, stderr = %q", got, stderr.String())
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cli", "status.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != string(want) {
		t.Fatalf("status output:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestStatusWWNTable(t *testing.T) {
	t.Parallel()

	backend := &enclosureStub{disks: fixtureDisks()}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"status", "--wwn"}); got != 0 {
		t.Fatalf("Run(status --wwn) = %d, stderr = %q", got, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	want := [][]string{
		{"POOL", "STATE", "DEVICE", "SERIAL", "WWN", "BAY", "BACKEND", "LED"},
		{"tank", "ONLINE", "/dev/sda", "SERIAL-A", "5000000000000001", "c0/e41/s1", "storcli", "off"},
		{"tank", "DEGRADED", "/dev/sdb", "SERIAL-B", "-", "c0/e41/s2", "storcli", "unknown"},
	}
	if len(lines) != len(want) {
		t.Fatalf("status output = %q", stdout.String())
	}
	for i, line := range lines {
		if got := strings.Fields(line); !reflect.DeepEqual(got, want[i]) {
			t.Fatalf("row %d = %v, want %v", i, got, want[i])
		}
	}
	if actions := backend.recordedActions(); len(actions) != 0 {
		t.Fatalf("status performed LED actions = %#v", actions)
	}
}

func TestStatusWWNEmpty(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: &enclosureStub{}, Stdout: &stdout}
	if got := runtime.Run(context.Background(), []string{"status", "--wwn"}); got != 0 {
		t.Fatalf("Run(status --wwn) = %d", got)
	}
	if got := stdout.String(); got != "No ZFS disks found.\n" {
		t.Fatalf("status output = %q", got)
	}
}

func TestStatusJSONSchema(t *testing.T) {
	t.Parallel()

	backend := &enclosureStub{disks: fixtureDisks(), diagnostics: []app.Diagnostic{{Code: app.ErrorUnsupported, Source: "ses", Message: "not mapped"}}}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"status", "--json"}); got != 0 {
		t.Fatalf("Run(status --json) = %d, stderr = %q", got, stderr.String())
	}
	var payload struct {
		SchemaVersion int              `json:"schema_version"`
		Disks         []app.Disk       `json:"disks"`
		Diagnostics   []app.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 1 || len(payload.Disks) != 2 || len(payload.Diagnostics) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Disks[0].WWN != "5000000000000001" {
		t.Fatalf("JSON WWN = %q", payload.Disks[0].WWN)
	}
	wantJSON := stdout.String()
	for _, args := range [][]string{{"status", "--json", "--wwn"}, {"status", "--wwn", "--json"}} {
		stdout.Reset()
		if got := runtime.Run(context.Background(), args); got != 0 {
			t.Fatalf("Run(%v) = %d", args, got)
		}
		if stdout.String() != wantJSON {
			t.Fatalf("--wwn changed JSON output: %q", stdout.String())
		}
	}
}

func TestLocateDryRunHasNoSideEffect(t *testing.T) {
	t.Parallel()

	backend := &enclosureStub{disks: fixtureDisks()}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"locate", "SERIAL-A", "--dry-run", "--timeout", "1s"}); got != 0 {
		t.Fatalf("Run(locate dry-run) = %d, stderr = %q", got, stderr.String())
	}
	if actions := backend.recordedActions(); len(actions) != 0 {
		t.Fatalf("dry-run actions = %#v", actions)
	}
	if !strings.Contains(stdout.String(), "c0/e41/s1") || !strings.Contains(stdout.String(), "auto-off after 1s") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestLocateTurnsOffAfterTimeout(t *testing.T) {
	t.Parallel()

	backend := &enclosureStub{disks: fixtureDisks()}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"locate", "c0/e41/s2", "--timeout=1ms"}); got != 0 {
		t.Fatalf("Run(locate) = %d, stderr = %q", got, stderr.String())
	}
	want := []app.LEDAction{app.LEDActionOn, app.LEDActionOff}
	if got := backend.recordedActions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %#v, want %#v", got, want)
	}
}

func TestLocateRejectsAmbiguousSerial(t *testing.T) {
	t.Parallel()

	disks := fixtureDisks()
	disks[1].Serial = disks[0].Serial
	backend := &enclosureStub{disks: disks}
	var stdout, stderr bytes.Buffer
	runtime := Runtime{Inventory: inventoryStub{}, Enclosures: backend, Stdout: &stdout, Stderr: &stderr}
	if got := runtime.Run(context.Background(), []string{"locate", "SERIAL-A", "--off"}); got != app.ExitCode(app.ErrorAmbiguous) {
		t.Fatalf("Run(ambiguous locate) = %d, stderr = %q", got, stderr.String())
	}
	if actions := backend.recordedActions(); len(actions) != 0 {
		t.Fatalf("ambiguous actions = %#v", actions)
	}
}

func TestHelpAndVersionAreSelfContained(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, "Usage: zfs-pay"},
		{[]string{"help", "locate"}, "TARGET"},
		{[]string{"help", "status"}, "--wwn"},
		{[]string{"status", "--help"}, "--wwn"},
		{[]string{"--version"}, "zfs-pay 1.2.3"},
	} {
		var stdout, stderr bytes.Buffer
		runtime := Runtime{Build: BuildInfo{Version: "1.2.3", Commit: "abc", Date: "today"}, Stdout: &stdout, Stderr: &stderr}
		if got := runtime.Run(context.Background(), test.args); got != 0 {
			t.Fatalf("Run(%v) = %d, stderr = %q", test.args, got, stderr.String())
		}
		if !strings.Contains(stdout.String(), test.want) {
			t.Fatalf("Run(%v) stdout = %q, want substring %q", test.args, stdout.String(), test.want)
		}
	}
}
