package automation

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
	"github.com/zoujunkun/zfs-pay/internal/state"
)

type fakeInventory struct {
	mu    sync.Mutex
	disks []app.Disk
	calls int
}

func (f *fakeInventory) Discover(context.Context) (inventory.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return inventory.Snapshot{Disks: append([]app.Disk(nil), f.disks...)}, nil
}

func (f *fakeInventory) set(disks []app.Disk) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disks = disks
}

type locateCall struct {
	bay    app.Bay
	action app.LEDAction
}

type fakeEnclosures struct {
	mu    sync.Mutex
	calls []locateCall
}

func (f *fakeEnclosures) Map(_ context.Context, disks []app.Disk) ([]app.Disk, []app.Diagnostic) {
	return append([]app.Disk(nil), disks...), nil
}

func (f *fakeEnclosures) Locate(_ context.Context, bay app.Bay, action app.LEDAction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, locateCall{bay: bay, action: action})
	return nil
}

func TestHandleFaultDuplicateAndOnline(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthFaulted, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	event := Event{ID: "20", Subclass: "statechange", Pool: "tank", GUID: "101", State: app.HealthFaulted, VdevType: "disk"}

	result, err := engine.Handle(context.Background(), event, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Action != app.LEDActionOn {
		t.Fatalf("unexpected fault result: %#v", result)
	}
	result, err = engine.Handle(context.Background(), event, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ignored || len(enclosures.calls) != 1 {
		t.Fatalf("duplicate event was not ignored: %#v calls=%#v", result, enclosures.calls)
	}

	inventoryService.set([]app.Disk{testDisk(app.HealthOnline, &bay)})
	event.ID = "21"
	event.State = app.HealthOnline
	result, err = engine.Handle(context.Background(), event, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Action != app.LEDActionOff || len(enclosures.calls) != 2 {
		t.Fatalf("unexpected online result: %#v calls=%#v", result, enclosures.calls)
	}
	data, err := engine.Store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.ManagedLEDs) != 0 {
		t.Fatalf("managed LED ownership was not cleared: %#v", data.ManagedLEDs)
	}
}

func TestHandleMissingDiskUsesVerifiedCache(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthOnline, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	if _, err := engine.Reconcile(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	inventoryService.set(nil)
	result, err := engine.Handle(context.Background(), Event{ID: "30", Pool: "tank", GUID: "101", State: app.HealthUnavail, VdevType: "disk"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || !result.Operations[0].UsedCache || result.Operations[0].Bay.ID() != bay.ID() {
		t.Fatalf("cached bay not used: %#v", result)
	}
}

func TestReconcileDoesNotClearManualLocate(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthOnline, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	result, err := engine.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 0 || len(enclosures.calls) != 0 {
		t.Fatalf("online unowned LED must remain untouched: %#v", result)
	}
}

func TestDryRunHasNoSideEffects(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthFaulted, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	result, err := engine.Handle(context.Background(), Event{Pool: "tank", GUID: "101", State: app.HealthFaulted, VdevType: "disk"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Applied || len(enclosures.calls) != 0 {
		t.Fatalf("dry run changed hardware: %#v", result)
	}
	data, err := engine.Store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Mappings) != 0 || len(data.ManagedLEDs) != 0 {
		t.Fatalf("dry run changed state: %#v", data)
	}
}

func TestNonDiskAndUnmanagedStateAreIgnored(t *testing.T) {
	inventoryService := &fakeInventory{}
	engine := testEngine(t, inventoryService, &fakeEnclosures{})
	result, err := engine.Handle(context.Background(), Event{VdevType: "mirror"}, false)
	if err != nil || !result.Ignored || inventoryService.calls != 0 {
		t.Fatalf("non-disk event: result=%#v err=%v calls=%d", result, err, inventoryService.calls)
	}
	result, err = engine.Handle(context.Background(), Event{Pool: "tank", GUID: "101", State: app.HealthOffline, VdevType: "disk"}, false)
	if err != nil || !result.Ignored || inventoryService.calls != 0 {
		t.Fatalf("offline event: result=%#v err=%v calls=%d", result, err, inventoryService.calls)
	}
}

func TestCurrentStateOverridesStaleFaultEvent(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthOnline, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	result, err := engine.Handle(context.Background(), Event{ID: "40", Pool: "tank", GUID: "101", State: app.HealthFaulted, VdevType: "disk"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ignored || len(enclosures.calls) != 0 {
		t.Fatalf("stale fault event should not switch LED on: %#v", result)
	}
}

func TestConcurrentDuplicateEventsOnlyLocateOnce(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthFaulted, &bay)}}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	event := Event{ID: "50", Subclass: "statechange", Pool: "tank", GUID: "101", State: app.HealthFaulted, VdevType: "disk"}
	var group sync.WaitGroup
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := engine.Handle(context.Background(), event, false); err != nil {
				t.Errorf("Handle: %v", err)
			}
		}()
	}
	group.Wait()
	if len(enclosures.calls) != 1 || enclosures.calls[0].action != app.LEDActionOn {
		t.Fatalf("locate calls = %#v", enclosures.calls)
	}
}

type blockingEnclosures struct{}

func (blockingEnclosures) Map(_ context.Context, disks []app.Disk) ([]app.Disk, []app.Diagnostic) {
	return disks, nil
}

func (blockingEnclosures) Locate(ctx context.Context, _ app.Bay, _ app.LEDAction) error {
	<-ctx.Done()
	return &app.Error{Code: app.ErrorTimeout, Op: "fake locate", Message: "timed out", Err: ctx.Err()}
}

func TestLocateTimeout(t *testing.T) {
	bay := testBay()
	inventoryService := &fakeInventory{disks: []app.Disk{testDisk(app.HealthFaulted, &bay)}}
	engine := testEngine(t, inventoryService, blockingEnclosures{})
	engine.Timeout = 5 * time.Millisecond
	_, err := engine.Handle(context.Background(), Event{Pool: "tank", GUID: "101", State: app.HealthFaulted, VdevType: "disk"}, false)
	var typed *app.Error
	if !errors.As(err, &typed) || typed.Code != app.ErrorTimeout {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestCleanupOnlyClearsManagedLEDsWithoutDiscovery(t *testing.T) {
	inventoryService := &fakeInventory{}
	enclosures := &fakeEnclosures{}
	engine := testEngine(t, inventoryService, enclosures)
	bay := testBay()
	if _, err := engine.Store.Update(context.Background(), func(data *state.Data) error {
		data.ManagedLEDs[managedKey(bay)] = state.ManagedLED{Pool: "tank", GUID: "101", Bay: bay}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Cleanup(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if inventoryService.calls != 0 || len(result.Operations) != 1 || result.Operations[0].Action != app.LEDActionOff {
		t.Fatalf("unexpected cleanup: result=%#v inventory calls=%d", result, inventoryService.calls)
	}
	data, err := engine.Store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.ManagedLEDs) != 0 {
		t.Fatalf("managed LEDs remain: %#v", data.ManagedLEDs)
	}
}

func testEngine(t *testing.T, inventoryService Inventory, enclosures Enclosures) Engine {
	t.Helper()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	return Engine{
		Inventory: inventoryService, Enclosures: enclosures,
		Store: state.Store{Path: filepath.Join(t.TempDir(), "state.json"), Now: func() time.Time { return now }},
		Now:   func() time.Time { return now },
	}
}

func testBay() app.Bay {
	return app.Bay{Backend: "fake", Controller: "c0", Enclosure: "e1", Slot: "s2", Capability: app.LEDCapabilityLocate}
}

func testDisk(health app.Health, bay *app.Bay) app.Disk {
	return app.Disk{Pool: "tank", VdevGUID: "101", VdevType: "data", State: health, DevicePath: "/dev/disk/by-id/wwn-test-101", Serial: "SERIAL-101", WWN: "5000000000000101", Bay: bay}
}
