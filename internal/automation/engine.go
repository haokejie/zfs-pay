package automation

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
	"github.com/zoujunkun/zfs-pay/internal/state"
)

type Inventory interface {
	Discover(context.Context) (inventory.Snapshot, error)
}

type Enclosures interface {
	Map(context.Context, []app.Disk) ([]app.Disk, []app.Diagnostic)
	Locate(context.Context, app.Bay, app.LEDAction) error
}

type Operation struct {
	GUID      string        `json:"guid"`
	Bay       app.Bay       `json:"bay"`
	Action    app.LEDAction `json:"action"`
	Applied   bool          `json:"applied"`
	UsedCache bool          `json:"used_cache,omitempty"`
}

type Result struct {
	Ignored        bool             `json:"ignored,omitempty"`
	Reason         string           `json:"reason,omitempty"`
	RecoveredState bool             `json:"recovered_state,omitempty"`
	Operations     []Operation      `json:"operations,omitempty"`
	Diagnostics    []app.Diagnostic `json:"diagnostics,omitempty"`
}

// Engine converts ZED state changes into the only supported mutation: locate
// LED on/off. All state-changing paths are serialized by Store.Update.
type Engine struct {
	Inventory  Inventory
	Enclosures Enclosures
	Store      state.Store
	Timeout    time.Duration
	Now        func() time.Time
	Logger     *slog.Logger
}

func (e Engine) Handle(ctx context.Context, event Event, dryRun bool) (Result, error) {
	if err := event.Validate(); err != nil {
		return Result{}, err
	}
	eventAction, relevant := actionFor(event.State)
	if event.VdevType != "disk" || !relevant {
		return Result{Ignored: true, Reason: "event does not target a managed disk state"}, nil
	}
	disks, diagnostics, err := e.discover(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Diagnostics: diagnostics}
	apply := func(data *state.Data) error {
		e.refreshMappings(data, disks)
		fingerprint := event.Fingerprint()
		if fingerprint != "" && data.LastEvents[event.GUID] == fingerprint {
			result.Ignored = true
			result.Reason = "duplicate event"
			return nil
		}
		disk := findDisk(disks, event.GUID)
		action := eventAction
		if disk != nil {
			currentAction, currentRelevant := actionFor(disk.State)
			if !currentRelevant {
				result.Ignored = true
				result.Reason = fmt.Sprintf("current vdev state %s is not managed", disk.State)
				if fingerprint != "" && !dryRun {
					data.LastEvents[event.GUID] = fingerprint
				}
				return nil
			}
			action = currentAction
		}
		if err := e.applyAction(ctx, data, event.Pool, event.GUID, disk, action, dryRun, &result); err != nil {
			return err
		}
		if fingerprint != "" && !dryRun {
			data.LastEvents[event.GUID] = fingerprint
		}
		return nil
	}
	if dryRun {
		data, err := e.Store.Read(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := apply(&data); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	recovered, err := e.Store.Update(ctx, apply)
	result.RecoveredState = recovered
	if err != nil {
		return result, err
	}
	e.logResult("event", event.Pool, event.GUID, result)
	return result, nil
}

// Reconcile refreshes healthy mappings and converges only LEDs already owned
// by this tool or required by a currently faulted disk.
func (e Engine) Reconcile(ctx context.Context, dryRun bool) (Result, error) {
	disks, diagnostics, err := e.discover(ctx)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].VdevGUID < disks[j].VdevGUID })
	result := Result{Diagnostics: diagnostics}
	apply := func(data *state.Data) error {
		e.refreshMappings(data, disks)
		for index := range disks {
			disk := &disks[index]
			action, relevant := actionFor(disk.State)
			if !relevant {
				continue
			}
			if err := e.applyAction(ctx, data, disk.Pool, disk.VdevGUID, disk, action, dryRun, &result); err != nil {
				return err
			}
		}
		return nil
	}
	if dryRun {
		data, err := e.Store.Read(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := apply(&data); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	recovered, err := e.Store.Update(ctx, apply)
	result.RecoveredState = recovered
	if err != nil {
		return result, err
	}
	e.logResult("reconcile", "", "", result)
	return result, nil
}

// Cleanup switches off only LEDs recorded in ManagedLEDs. It deliberately does
// not discover disks, so package removal can run even when ZFS is unavailable.
func (e Engine) Cleanup(ctx context.Context, dryRun bool) (Result, error) {
	result := Result{}
	apply := func(data *state.Data) error {
		keys := make([]string, 0, len(data.ManagedLEDs))
		for key := range data.ManagedLEDs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			managed := data.ManagedLEDs[key]
			operation := Operation{GUID: managed.GUID, Bay: managed.Bay, Action: app.LEDActionOff, Applied: !dryRun}
			if !dryRun {
				if err := e.locate(ctx, managed.Bay, app.LEDActionOff); err != nil {
					return err
				}
				delete(data.ManagedLEDs, key)
			}
			result.Operations = append(result.Operations, operation)
		}
		if len(keys) == 0 {
			result.Ignored = true
			result.Reason = "no managed locate LEDs require clearing"
		}
		return nil
	}
	if dryRun {
		data, err := e.Store.Read(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := apply(&data); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	recovered, err := e.Store.Update(ctx, apply)
	result.RecoveredState = recovered
	if err != nil {
		return result, err
	}
	e.logResult("cleanup", "", "", result)
	return result, nil
}

func (e Engine) discover(ctx context.Context) ([]app.Disk, []app.Diagnostic, error) {
	if e.Inventory == nil || e.Enclosures == nil {
		return nil, nil, &app.Error{Code: app.ErrorInternal, Op: "automation discover", Message: "inventory or enclosure service is unavailable"}
	}
	callCtx, cancel := context.WithTimeout(ctx, e.timeout())
	defer cancel()
	snapshot, err := e.Inventory.Discover(callCtx)
	if err != nil {
		return nil, nil, err
	}
	disks, diagnostics := e.Enclosures.Map(callCtx, snapshot.Disks)
	diagnostics = append(snapshot.Diagnostics, diagnostics...)
	return disks, diagnostics, nil
}

func (e Engine) refreshMappings(data *state.Data, disks []app.Disk) {
	now := e.now()
	for _, disk := range disks {
		if disk.VdevGUID == "" || disk.Bay == nil || disk.Bay.ID() == "" || (disk.Serial == "" && disk.WWN == "") {
			continue
		}
		data.Mappings[disk.VdevGUID] = state.Mapping{
			Pool: disk.Pool, GUID: disk.VdevGUID, Serial: disk.Serial, WWN: disk.WWN,
			Bay: *disk.Bay, LastVerified: now,
		}
	}
}

func (e Engine) applyAction(ctx context.Context, data *state.Data, pool, guid string, disk *app.Disk, action app.LEDAction, dryRun bool, result *Result) error {
	if action == app.LEDActionOff {
		keys := managedKeys(data, guid)
		for _, key := range keys {
			managed := data.ManagedLEDs[key]
			operation := Operation{GUID: guid, Bay: managed.Bay, Action: action, Applied: !dryRun}
			if !dryRun {
				if err := e.locate(ctx, managed.Bay, action); err != nil {
					return err
				}
				delete(data.ManagedLEDs, key)
			}
			result.Operations = append(result.Operations, operation)
		}
		if len(keys) == 0 {
			result.Ignored = true
			result.Reason = "no managed locate LED requires clearing"
		}
		return nil
	}

	bay, usedCache, err := targetBay(data, guid, disk)
	if err != nil {
		return err
	}
	key := managedKey(bay)
	if managed, ok := data.ManagedLEDs[key]; ok {
		if managed.GUID != guid {
			return &app.Error{Code: app.ErrorAmbiguous, Op: "automation locate", Message: "target bay is owned by another vdev"}
		}
		result.Ignored = true
		result.Reason = "locate LED is already managed on"
		return nil
	}
	operation := Operation{GUID: guid, Bay: bay, Action: action, Applied: !dryRun, UsedCache: usedCache}
	if !dryRun {
		if err := e.locate(ctx, bay, action); err != nil {
			return err
		}
		data.ManagedLEDs[key] = state.ManagedLED{Pool: pool, GUID: guid, Bay: bay, Since: e.now()}
	}
	result.Operations = append(result.Operations, operation)
	return nil
}

func targetBay(data *state.Data, guid string, disk *app.Disk) (app.Bay, bool, error) {
	if disk != nil && disk.Bay != nil && disk.Bay.ID() != "" {
		return *disk.Bay, false, nil
	}
	if mapping, ok := data.Mappings[guid]; ok && mapping.GUID == guid && mapping.Bay.ID() != "" {
		return mapping.Bay, true, nil
	}
	return app.Bay{}, false, &app.Error{Code: app.ErrorNotFound, Op: "automation locate", Message: "no verified physical bay is available for vdev"}
}

func (e Engine) locate(ctx context.Context, bay app.Bay, action app.LEDAction) error {
	callCtx, cancel := context.WithTimeout(ctx, e.timeout())
	defer cancel()
	return e.Enclosures.Locate(callCtx, bay, action)
}

func actionFor(health app.Health) (app.LEDAction, bool) {
	switch health {
	case app.HealthDegraded, app.HealthFaulted, app.HealthUnavail, app.HealthRemoved:
		return app.LEDActionOn, true
	case app.HealthOnline:
		return app.LEDActionOff, true
	default:
		return "", false
	}
}

func findDisk(disks []app.Disk, guid string) *app.Disk {
	for index := range disks {
		if disks[index].VdevGUID == guid {
			return &disks[index]
		}
	}
	return nil
}

func managedKey(bay app.Bay) string { return bay.Backend + ":" + bay.ID() }

func managedKeys(data *state.Data, guid string) []string {
	var keys []string
	for key, managed := range data.ManagedLEDs {
		if managed.GUID == guid {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (e Engine) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return 20 * time.Second
}

func (e Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e Engine) logResult(kind, pool, guid string, result Result) {
	if e.Logger == nil {
		return
	}
	e.Logger.Info("automation completed", "kind", kind, "pool", pool, "guid", guid, "operations", len(result.Operations), "ignored", result.Ignored, "recovered_state", result.RecoveredState)
}
