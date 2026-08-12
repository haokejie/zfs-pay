package app

import "fmt"

// Health is the normalized OpenZFS state for a pool or leaf vdev.
type Health string

const (
	HealthUnknown  Health = "UNKNOWN"
	HealthOnline   Health = "ONLINE"
	HealthDegraded Health = "DEGRADED"
	HealthFaulted  Health = "FAULTED"
	HealthOffline  Health = "OFFLINE"
	HealthRemoved  Health = "REMOVED"
	HealthUnavail  Health = "UNAVAIL"
	HealthAvail    Health = "AVAIL"
	HealthInUse    Health = "INUSE"
)

// LEDCapability describes the only mutable hardware feature exposed by zfs-pay.
type LEDCapability string

const (
	LEDCapabilityUnknown     LEDCapability = "unknown"
	LEDCapabilityUnsupported LEDCapability = "unsupported"
	LEDCapabilityLocate      LEDCapability = "locate"
)

// LEDState is the last observed or managed locate LED state.
type LEDState string

const (
	LEDStateUnknown LEDState = "unknown"
	LEDStateOff     LEDState = "off"
	LEDStateOn      LEDState = "on"
)

// LEDAction is deliberately limited to locate LED on/off.
type LEDAction string

const (
	LEDActionOn  LEDAction = "on"
	LEDActionOff LEDAction = "off"
)

// ErrorCounters are the error totals reported by OpenZFS for one vdev.
type ErrorCounters struct {
	Read     uint64 `json:"read"`
	Write    uint64 `json:"write"`
	Checksum uint64 `json:"checksum"`
}

// Bay identifies one physical slot without exposing a vendor-specific command.
type Bay struct {
	Backend    string        `json:"backend"`
	Controller string        `json:"controller"`
	Enclosure  string        `json:"enclosure"`
	Slot       string        `json:"slot"`
	Capability LEDCapability `json:"capability"`
	LEDState   LEDState      `json:"led_state"`
}

func (b Bay) ID() string {
	if b.Controller == "" || b.Enclosure == "" || b.Slot == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", b.Controller, b.Enclosure, b.Slot)
}

// Disk is a normalized ZFS leaf vdev and its optional physical location.
type Disk struct {
	Pool         string        `json:"pool"`
	VdevGUID     string        `json:"vdev_guid"`
	VdevName     string        `json:"vdev_name"`
	VdevType     string        `json:"vdev_type"`
	State        Health        `json:"state"`
	DevicePath   string        `json:"device_path,omitempty"`
	ParentDevice string        `json:"parent_device,omitempty"`
	Serial       string        `json:"serial,omitempty"`
	WWN          string        `json:"wwn,omitempty"`
	Model        string        `json:"model,omitempty"`
	Errors       ErrorCounters `json:"errors"`
	Bay          *Bay          `json:"bay,omitempty"`
	Diagnostics  []Diagnostic  `json:"diagnostics,omitempty"`
}

// Diagnostic preserves partial-discovery problems without hiding healthy disks.
type Diagnostic struct {
	Code    ErrorCode `json:"code"`
	Source  string    `json:"source,omitempty"`
	Message string    `json:"message"`
}
