# CLI

## Status

```sh
zfs-pay status
zfs-pay status --json
```

The text view is intentionally compact. JSON output uses `schema_version: 1`
and preserves global and per-disk diagnostics when part of discovery is
unsupported.

## Locate

```sh
zfs-pay locate /dev/sdb --dry-run
zfs-pay locate SERIAL --timeout 60s
zfs-pay locate c0/e41/s2 --off
```

Targets must match exactly one disk by device path, serial, ZFS GUID, or full
backend bay ID. The command never accepts arbitrary StorCLI or ledctl arguments.
An enabled locate LED is turned off after 60 seconds by default; cancellation
also attempts a bounded cleanup before exiting.
