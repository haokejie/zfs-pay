# Security model

## Trust boundary

`zfs-pay` reads ZFS topology, Linux block identity, and enclosure inventory from local system tools. Root is required for some controller and sysfs operations. CLI arguments, ZED environment fields, external command output, and the persistent state file are treated as untrusted input.

## Mutation boundary

The enclosure interface exposes only two enumerated actions: locate on and locate off. Backends construct commands from validated controller, enclosure, slot, or block-device components. The project has no API for changing ZFS membership, disk online state, rebuild state, firmware, data, or RAID configuration.

Manual `locate` turns a selected light off after the requested timeout. Automation separately records LED ownership in its versioned state. Reconciliation and uninstall cleanup only turn off entries from that managed set.

## Failure behavior

- Missing identity, duplicate identity, incomplete bay IDs, malformed output, unsupported tools, and topology changes fail closed.
- External commands use argv arrays without shell interpolation and run under bounded contexts.
- State updates use an exclusive file lock and an fsync/rename sequence. Corrupt state is preserved with a `.corrupt-*` suffix before clean recovery.
- A missing disk may use only a GUID-to-bay mapping previously verified while identity was live.
- `ledctl` is disabled unless explicitly enabled.

## Logs and state

Logs may contain a pool name, vdev GUID, backend, bay ID, action, and result. They must not contain credentials or full raw inventory output. State is created with mode `0600` under a `0750` directory and contains no credentials.

## Deployment guidance

1. Verify the package checksum before installation.
2. Run `zfs-pay status` and inspect every mapped bay before enabling automatic events.
3. Use `locate --dry-run` before the first controlled locate test.
4. Keep existing ZFS/PVE alerts, SMART monitoring, backups, and recovery procedures. This tool does not replace them.
5. Leave `ledctl` disabled unless its behavior has been confirmed on the exact controller and backplane.
