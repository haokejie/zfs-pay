package automation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

var decimalGUID = regexp.MustCompile(`^[0-9]+$`)

// Event is the small, validated subset of the ZED environment used by zfs-pay.
type Event struct {
	ID       string     `json:"id,omitempty"`
	Subclass string     `json:"subclass,omitempty"`
	Pool     string     `json:"pool"`
	GUID     string     `json:"vdev_guid"`
	Path     string     `json:"vdev_path,omitempty"`
	State    app.Health `json:"vdev_state"`
	VdevType string     `json:"vdev_type"`
}

// EventFromLookup avoids importing the complete process environment into logs
// or state while remaining straightforward to test.
func EventFromLookup(lookup func(string) string) (Event, error) {
	event := Event{
		ID:       strings.TrimSpace(lookup("ZEVENT_EID")),
		Subclass: strings.TrimSpace(lookup("ZEVENT_SUBCLASS")),
		Pool:     strings.TrimSpace(lookup("ZEVENT_POOL")),
		GUID:     strings.TrimSpace(lookup("ZEVENT_VDEV_GUID")),
		Path:     strings.TrimSpace(lookup("ZEVENT_VDEV_PATH")),
		State:    app.Health(strings.ToUpper(strings.TrimSpace(lookup("ZEVENT_VDEV_STATE_STR")))),
		VdevType: strings.ToLower(strings.TrimSpace(lookup("ZEVENT_VDEV_TYPE"))),
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (e Event) Validate() error {
	if e.VdevType == "" {
		return malformedEvent("ZEVENT_VDEV_TYPE is required")
	}
	if e.VdevType != "disk" {
		return nil
	}
	if e.Pool == "" {
		return malformedEvent("ZEVENT_POOL is required for disk events")
	}
	if !decimalGUID.MatchString(e.GUID) {
		return malformedEvent("ZEVENT_VDEV_GUID must be a decimal GUID")
	}
	if e.State == "" {
		return malformedEvent("ZEVENT_VDEV_STATE_STR is required for disk events")
	}
	return nil
}

func (e Event) Fingerprint() string {
	if e.ID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", e.Subclass, e.ID)
}

func malformedEvent(message string) error {
	return &app.Error{Code: app.ErrorMalformed, Op: "parse ZED event", Message: message}
}
