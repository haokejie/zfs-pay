# CLI

## Status

```sh
zfs-pay status
zfs-pay status --wwn
zfs-pay status --json
```

The default table is unchanged. `--wwn` adds the full WWN after SERIAL, with `-`
for missing values. Identifiers are not truncated; the extra column may wrap in
narrow terminals. `--wwn` has no effect with `--json`, which already includes WWN.
JSON output uses `schema_version: 1`
and preserves global and per-disk diagnostics when part of discovery is
unsupported.

## Locate

Read the target disk's BAY from `status` or `status --wwn`; replace the example
bay below with that value. Preview the operation, then light the LED for 10 minutes:

```sh
zfs-pay locate c0/e10/s2 --dry-run
zfs-pay locate c0/e10/s2 --timeout 10m
```

The command waits in the foreground until the timeout; keep the session open.
Press `Ctrl+C` to stop early, or turn the LED off from another terminal:

```sh
zfs-pay locate c0/e10/s2 --off
```

Targets must match exactly one disk by device path, serial, ZFS GUID, or full
backend bay ID. The command never accepts arbitrary StorCLI or ledctl arguments.
An enabled locate LED is turned off after 60 seconds by default. The maximum
timeout is 24 hours; cancellation also attempts a bounded cleanup before exiting.

Lighting the LED does not detach or offline a disk. To permanently remove a mirror
member, identify and record its physical bay first, detach the intended member
using ZFS administration tools, and verify the remaining mirror is healthy before
unplugging it. Detached disks may no longer be selectable by `locate`; `zfs-pay`
does not perform ZFS detach or offline operations.
