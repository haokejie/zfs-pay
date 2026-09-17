# zfs-pay

English | [简体中文](README.zh-CN.md)

`zfs-pay` maps OpenZFS leaf vdevs to Linux block devices and physical drive bays. It gives Proxmox VE and Debian administrators one command to inspect disk locations, safely control locate LEDs, and optionally keep fault LEDs in sync through ZED.

It discovers mappings dynamically from ZFS, `lsblk`/udev, and enclosure or controller data. No handwritten bay table is required.

> [!IMPORTANT]
> The only hardware mutation exposed by `zfs-pay` is locate LED on/off. It cannot offline, replace, rebuild, initialize, erase, flash, or reconfigure a disk, ZFS pool, or RAID controller.

## Features

- Shows ZFS pool, vdev state, Linux device, serial number, physical bay, backend, and LED state.
- Supports compact terminal output and versioned JSON output.
- Locates exactly one disk by device path, serial number, ZFS GUID, or full bay ID.
- Fails closed when identity is missing or ambiguous.
- Supports StorCLI-compatible controllers and Linux SES, with `ledctl` available as an explicit opt-in.
- Uses ZED to locate faulted disks and a verified cache to retain the last known bay after a disk disappears.
- Automatically turns off only LEDs that were switched on by `zfs-pay` automation.
- Ships as reproducible Debian packages for `amd64` and `arm64`.

## Requirements

- Proxmox VE or Debian Linux with OpenZFS.
- Root access for installation and most enclosure LED operations.
- At least one usable bay backend:

| Backend | Availability | Notes |
| --- | --- | --- |
| StorCLI-compatible | Auto-detected | Recommended for supported Broadcom/LSI MegaRAID controllers. StorCLI/PERCCLI must be installed separately. |
| Linux SES sysfs | Auto-detected | Requires an exact block-device-to-enclosure mapping exposed by the kernel. |
| `ledctl` | Disabled by default | Enable only after validating the exact controller and backplane. |

StorCLI and PERCCLI are proprietary tools and are never included in this project or its packages. Installation still succeeds when no LED backend is available; `status` reports the missing capability instead of guessing a bay.

## Install

Download the `.deb` for the target architecture and verify it against `SHA256SUMS`:

```sh
sha256sum -c SHA256SUMS --ignore-missing
apt install ./zfs-pay_VERSION_ARCH.deb
```

Verify the installation:

```sh
zfs-pay --version
zfs-pay status
systemctl is-enabled zfs-pay-reconcile.service
systemctl is-active zfs-zed
```

The package installs:

| Path | Purpose |
| --- | --- |
| `/usr/bin/zfs-pay` | User-facing command |
| `/usr/lib/zfs-pay/zfs-pay-zed` | Private ZED/systemd helper |
| `/etc/zfs/zed.d/*-zfs-pay.sh` | ZED hook links |
| `/usr/lib/systemd/system/zfs-pay-reconcile.service` | Startup reconciliation |
| `/etc/default/zfs-pay` | Automation configuration |
| `/var/lib/zfs-pay/state.json` | Verified bay cache and managed LED state |

Installation or upgrade reloads systemd and may briefly restart `zfs-zed` so the new hooks are picked up. It does not restart ZFS pools or virtual machines and does not change disk state.

## Check Drive Bays

```sh
zfs-pay status
```

Example:

```text
POOL  STATE   DEVICE   SERIAL         BAY         BACKEND  LED
tank  ONLINE  /dev/sda TEST-SERIAL-A  c0/e10/s1  storcli  unknown
tank  ONLINE  /dev/sdb TEST-SERIAL-B  c0/e10/s2  storcli  unknown
```

The bay format is `controller/enclosure/slot`. For example, `c0/e10/s2` means controller 0, enclosure 10, slot 2.

To add the full WWN after SERIAL while keeping all existing columns:

```sh
zfs-pay status --wwn
```

SERIAL is the manufacturer's serial number; WWN is the worldwide storage identifier. Missing WWNs display as `-`. The default table stays compact; `--wwn` may wrap in narrow terminals, but identifiers are never truncated. JSON already includes WWN, so combining `--wwn` with `--json` does not change its output.

Use JSON for scripts and monitoring integrations:

```sh
zfs-pay status --json
zfs-pay status --json | jq '.disks[] | {pool, state, vdev_guid, device_path, bay}'
```

JSON output currently uses `schema_version: 1`. Global and per-disk diagnostics are retained even when another disk was discovered successfully.

## Locate a Drive

Run `zfs-pay status` (or `status --wwn`) and use the target disk's BAY value.
The `c0/e10/s2` below is an example, not a fixed bay for every server. Always
inspect the plan before the first locate operation on a server:

```sh
zfs-pay locate c0/e10/s2 --dry-run
```

Turn the locate LED on for 60 seconds. The command waits, then turns it off automatically:

```sh
zfs-pay locate c0/e10/s2
```

For a physical inspection, keep the LED on for 10 minutes:

```sh
zfs-pay locate c0/e10/s2 --timeout 10m
```

The command stays in the foreground while the LED is on; this is expected, not a hang. Keep the terminal session open. The default duration is 60 seconds and the maximum is 24 hours. Interrupting the command with `Ctrl+C` also attempts to turn the LED off before exiting.

To turn the LED off early, press `Ctrl+C` in the original terminal or run this in another terminal:

```sh
zfs-pay locate c0/e10/s2 --off
```

`TARGET` must match exactly one disk:

```sh
zfs-pay locate /dev/sdb
zfs-pay locate /dev/sdb1
zfs-pay locate TEST-SERIAL-B
zfs-pay locate 7100000000000002
zfs-pay locate c0/e10/s2
```

The examples represent a parent disk, leaf device, serial number, ZFS vdev GUID, and full bay ID. If a value matches more than one disk, use the GUID or full bay ID.

### Before Removing a Drive

A locate LED only identifies the physical drive. It does not detach or offline the disk, and does not mean it is ready to unplug. Before permanently removing a member from a mirror, verify and record its identity and physical bay, detach the intended member using ZFS administration tools, and confirm the remaining mirror is healthy before unplugging it. `zfs-pay` does not perform these ZFS operations. A detached disk is no longer a pool member and may no longer be selectable by `zfs-pay locate`, so identify it before detaching.

## Automatic Fault LEDs

The Debian package enables a oneshot reconciliation service and installs ZED hooks for disk state changes, pool import, vdev attach/clear, and startup.

For disk vdevs, automation handles these states:

| ZFS state | Action |
| --- | --- |
| `DEGRADED`, `FAULTED`, `UNAVAIL`, `REMOVED` | Turn the verified bay's locate LED on |
| `ONLINE` | Turn off the LED only when it is managed by `zfs-pay` |
| Other states or non-disk vdevs | Ignore safely |

Run reconciliation manually after checking current pool health:

```sh
zpool status -x
systemctl start zfs-pay-reconcile.service
systemctl status zfs-pay-reconcile.service --no-pager
```

Inspect automation logs:

```sh
journalctl -u zfs-pay-reconcile.service
journalctl -u zfs-zed --since today
```

Do not inject a real disk fault, offline a disk, or pull a drive just to test automation. Use `locate --dry-run` and the project's fixture tests instead.

## Configuration

Automation reads `/etc/default/zfs-pay`:

```sh
# Maximum duration for one discovery or backend command.
ZFS_PAY_COMMAND_TIMEOUT=20s

# Disabled unless explicitly enabled after hardware validation.
# ZFS_PAY_ENABLE_LEDCTL=1
```

After changing the file, run reconciliation to validate the configuration:

```sh
systemctl start zfs-pay-reconcile.service
```

For an interactive command to use the same environment, load it into the current shell first:

```sh
set -a
. /etc/default/zfs-pay
set +a
zfs-pay status
```

## Troubleshooting

Start with read-only checks:

```sh
zpool status -x
zfs-pay status
zfs-pay status --json
systemctl status zfs-zed --no-pager
systemctl status zfs-pay-reconcile.service --no-pager
journalctl -u zfs-pay-reconcile.service -n 50 --no-pager
```

Common diagnostics:

| Diagnostic | Meaning |
| --- | --- |
| `unsupported` | A tool or enclosure backend is unavailable. This is non-fatal when another backend maps the disk. |
| `permission_denied` | Run with the required privileges and check device or sysfs permissions. |
| `not_found` | The target disk or a verified physical bay was not found. |
| `ambiguous` | More than one disk or slot matched. Use an exact GUID or full bay ID. |
| `timeout` | A ZFS, Linux, or controller command exceeded its bounded timeout. |
| `malformed_output` | An external tool returned output that could not be safely interpreted. |

Useful controller checks:

```sh
command -v storcli64
command -v storcli
test -x /opt/MegaRAID/storcli/storcli64 && echo "StorCLI found"
```

Do not create a manual serial-to-slot table to work around a failed mapping. Fix the identity or backend visibility so the mapping remains verifiable after disks are replaced or reordered.

## Uninstall

Remove the command, service, and ZED hooks while retaining configuration and cached mappings:

```sh
apt remove zfs-pay
```

Before removal, the package makes a best-effort attempt to turn off only locate LEDs recorded as managed by `zfs-pay`.

Remove configuration and cached state as well:

```sh
apt purge zfs-pay
```

Neither operation changes ZFS pool membership, vdev state, rebuild state, firmware, or RAID configuration.

## Build from Source

The project uses Go 1.24 and currently has no third-party Go module dependencies.

```sh
make verify
go test -race ./...
make build-linux
make package PACKAGE_VERSION=0.1.0~dev
make package-test
```

Build output is written to the ignored `dist/` directory. Docker Buildx creates reproducible Debian packages for `amd64` and `arm64`.

## More Documentation

- [CLI contract](docs/cli.md)
- [Installation details](docs/install.md)
- [Compatibility evidence](docs/compatibility.md)
- [Security model](docs/security.md)
- [Release process](docs/release.md)
- [Contributing](CONTRIBUTING.md)
- [Security reporting](SECURITY.md)

## License

MIT. See [LICENSE](LICENSE).
