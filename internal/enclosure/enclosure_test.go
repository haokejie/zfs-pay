package enclosure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

type fakeBackend struct {
	name    string
	slots   []Slot
	err     error
	located []app.Bay
	delay   time.Duration
	mu      sync.Mutex
	active  int
	max     int
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) List(context.Context, []app.Disk) ([]Slot, error) {
	return f.slots, f.err
}
func (f *fakeBackend) Locate(_ context.Context, bay app.Bay, _ app.LEDAction) error {
	f.mu.Lock()
	f.active++
	if f.active > f.max {
		f.max = f.active
	}
	f.mu.Unlock()
	time.Sleep(f.delay)
	f.mu.Lock()
	f.located = append(f.located, bay)
	f.active--
	f.mu.Unlock()
	return f.err
}

func TestServiceMapsBySerialInBackendPriority(t *testing.T) {
	t.Parallel()

	first := &fakeBackend{name: "first", slots: []Slot{{Bay: app.Bay{Backend: "first", Controller: "c0", Enclosure: "e1", Slot: "s1"}, Serial: " disk-a "}}}
	second := &fakeBackend{name: "second", slots: []Slot{{Bay: app.Bay{Backend: "second", Controller: "c1", Enclosure: "e2", Slot: "s2"}, Serial: "DISK-A"}}}
	disks, diagnostics := (&Service{Backends: []Backend{first, second}}).Map(context.Background(), []app.Disk{{Serial: "Disk-A"}})
	if len(diagnostics) != 0 || len(disks) != 1 || disks[0].Bay == nil || disks[0].Bay.Backend != "first" {
		t.Fatalf("disks = %#v, diagnostics = %#v", disks, diagnostics)
	}
}

func TestServiceUsesWWNToDisambiguateDuplicateSerial(t *testing.T) {
	t.Parallel()

	backend := &fakeBackend{name: "storcli", slots: []Slot{
		{Bay: app.Bay{Backend: "storcli", Slot: "s1"}, Serial: "DUP", WWN: "0x1001"},
		{Bay: app.Bay{Backend: "storcli", Slot: "s2"}, Serial: "DUP", WWN: "0x1002"},
	}}
	disks, _ := (&Service{Backends: []Backend{backend}}).Map(context.Background(), []app.Disk{{Serial: "dup", WWN: "1002"}})
	if disks[0].Bay == nil || disks[0].Bay.Slot != "s2" {
		t.Fatalf("mapped disk = %#v", disks[0])
	}
}

func TestServiceReportsBackendFailureAndUnknownLocate(t *testing.T) {
	t.Parallel()

	backend := &fakeBackend{name: "bad", err: &app.Error{Code: app.ErrorUnsupported, Message: "missing"}}
	_, diagnostics := (&Service{Backends: []Backend{backend}}).Map(context.Background(), nil)
	if len(diagnostics) != 1 || diagnostics[0].Code != app.ErrorUnsupported {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	err := (&Service{}).Locate(context.Background(), app.Bay{Backend: "missing"}, app.LEDActionOn)
	var typed *app.Error
	if !errors.As(err, &typed) || typed.Code != app.ErrorUnsupported {
		t.Fatalf("error = %#v", err)
	}
}

func TestServicePlanIsSideEffectFreeAndLocateSerializesTarget(t *testing.T) {
	t.Parallel()

	backend := &fakeBackend{name: "storcli", delay: 15 * time.Millisecond}
	service := &Service{Backends: []Backend{backend}}
	bay := app.Bay{Backend: "storcli", Controller: "c0", Enclosure: "e1", Slot: "s2"}
	plan, err := service.Plan(bay, app.LEDActionOn)
	if err != nil || plan.Target != "c0/e1/s2" || len(backend.located) != 0 {
		t.Fatalf("plan = %#v, error = %v, located = %#v", plan, err, backend.located)
	}

	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := service.Locate(context.Background(), bay, app.LEDActionOn); err != nil {
				t.Errorf("Locate() error = %v", err)
			}
		}()
	}
	wait.Wait()
	if backend.max != 1 || len(backend.located) != 2 {
		t.Fatalf("max concurrent = %d, located = %d", backend.max, len(backend.located))
	}
}
