package enclosure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

// Slot is a backend-neutral physical location and its installed disk identity.
type Slot struct {
	Bay    app.Bay
	Serial string
	WWN    string
	Model  string
	Device string
}

// Backend exposes only enumeration and locate LED control.
type Backend interface {
	Name() string
	List(context.Context, []app.Disk) ([]Slot, error)
	Locate(context.Context, app.Bay, app.LEDAction) error
}

// Service joins inventory disks to backends in priority order.
type Service struct {
	Backends []Backend
	locks    sync.Map
}

func (s *Service) Map(ctx context.Context, input []app.Disk) ([]app.Disk, []app.Diagnostic) {
	disks := append([]app.Disk(nil), input...)
	var diagnostics []app.Diagnostic
	for _, backend := range s.Backends {
		if backend == nil {
			continue
		}
		slots, err := backend.List(ctx, disks)
		if err != nil {
			diagnostics = append(diagnostics, diagnosticFromError(backend.Name(), "backend unavailable", err))
			continue
		}
		for i := range disks {
			if disks[i].Bay != nil {
				continue
			}
			matches := matchSlots(disks[i], slots)
			switch len(matches) {
			case 0:
				continue
			case 1:
				bay := matches[0].Bay
				disks[i].Bay = &bay
			default:
				disks[i].Diagnostics = append(disks[i].Diagnostics, app.Diagnostic{
					Code:    app.ErrorAmbiguous,
					Source:  backend.Name(),
					Message: "disk identity matches multiple enclosure slots",
				})
			}
		}
	}
	return disks, diagnostics
}

// LocatePlan is a side-effect-free description used by CLI dry-run output.
type LocatePlan struct {
	Backend string        `json:"backend"`
	Target  string        `json:"target"`
	Action  app.LEDAction `json:"action"`
}

func (p LocatePlan) String() string {
	return fmt.Sprintf("%s %s locate=%s", p.Backend, p.Target, p.Action)
}

func (s *Service) Plan(bay app.Bay, action app.LEDAction) (LocatePlan, error) {
	if action != app.LEDActionOn && action != app.LEDActionOff {
		return LocatePlan{}, &app.Error{Code: app.ErrorUnsupported, Op: "locate", Message: "unsupported LED action"}
	}
	for _, backend := range s.Backends {
		if backend != nil && backend.Name() == bay.Backend {
			if bay.ID() == "" {
				return LocatePlan{}, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "incomplete enclosure target"}
			}
			return LocatePlan{Backend: backend.Name(), Target: bay.ID(), Action: action}, nil
		}
	}
	return LocatePlan{}, &app.Error{Code: app.ErrorUnsupported, Op: "locate", Message: "enclosure backend is unavailable"}
}

func (s *Service) Locate(ctx context.Context, bay app.Bay, action app.LEDAction) error {
	plan, err := s.Plan(bay, action)
	if err != nil {
		return err
	}
	value, _ := s.locks.LoadOrStore(plan.Backend+":"+plan.Target, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	for _, backend := range s.Backends {
		if backend != nil && backend.Name() == plan.Backend {
			return backend.Locate(ctx, bay, action)
		}
	}
	return &app.Error{Code: app.ErrorUnsupported, Op: "locate", Message: "enclosure backend is unavailable"}
}

func matchSlots(disk app.Disk, slots []Slot) []Slot {
	serial := NormalizeIdentity(disk.Serial)
	wwn := NormalizeWWN(disk.WWN)
	var matches []Slot
	if serial != "" {
		for _, slot := range slots {
			if NormalizeIdentity(slot.Serial) == serial {
				matches = append(matches, slot)
			}
		}
		if len(matches) > 1 && wwn != "" {
			if filtered := filterByWWN(matches, wwn); len(filtered) > 0 {
				matches = filtered
			}
		}
		return matches
	}
	if wwn != "" {
		return filterByWWN(slots, wwn)
	}
	return nil
}

func filterByWWN(slots []Slot, wwn string) []Slot {
	var matches []Slot
	for _, slot := range slots {
		if NormalizeWWN(slot.WWN) == wwn {
			matches = append(matches, slot)
		}
	}
	return matches
}

func NormalizeIdentity(value string) string {
	return strings.ToUpper(strings.Trim(strings.TrimSpace(value), "\x00"))
}

func NormalizeWWN(value string) string {
	return strings.TrimPrefix(NormalizeIdentity(value), "0X")
}

func diagnosticFromError(source, message string, err error) app.Diagnostic {
	code := app.ErrorInternal
	var typed *app.Error
	if errors.As(err, &typed) {
		code = typed.Code
	}
	return app.Diagnostic{Code: code, Source: source, Message: message}
}
