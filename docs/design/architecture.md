# Architecture

`zfs-pay` is a local, short-lived CLI plus a narrow ZED event adapter. It does
not open a network port.

## Layers

1. `internal/zfs`, `internal/system`, and `internal/inventory` discover ZFS
   leaf vdevs and stable Linux disk identity.
2. `internal/enclosure` selects a backend and joins disks to controller,
   enclosure, and slot records by an exact normalized identity.
3. Vendor-specific code lives under `internal/backends`; it may enumerate bays
   and turn only the locate LED on or off.
4. `internal/cli` renders status and validates an unambiguous locate target.
5. `internal/automation` and `internal/state` process ZED events and keep the
   last verified GUID-to-bay mapping for a disk that later disappears.

## Safety boundary

There is no generic controller command interface. Backends implement a typed
locate action, and malformed, missing, unsupported, or ambiguous discovery
results fail closed. Tests use command runners and anonymized fixtures instead
of real storage hardware.
