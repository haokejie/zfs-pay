# Installation

## Supported packages

The first release targets Debian and Proxmox VE on `amd64` and `arm64`. Download the matching `.deb` and verify it against `SHA256SUMS` before installation.

```sh
sha256sum -c SHA256SUMS --ignore-missing
apt install ./zfs-pay_VERSION_ARCH.deb
```

The package installs `zfs-pay` in `/usr/bin`, the private ZED helper and zedlets under `/usr/lib/zfs-pay`, zedlet links under `/etc/zfs/zed.d`, and a startup reconcile unit. Installation does not change a pool, offline a disk, or configure a controller.

## Controller support

StorCLI and PERCCLI are proprietary and are never included in the package. Install the controller utility supplied for the server separately. Without StorCLI or a usable SES mapping, installation still succeeds and `zfs-pay status` reports the unavailable backend as a diagnostic.

`ledctl` is disabled by default. Enable it only after validating the enclosure behavior:

```sh
sed -i 's/^# ZFS_PAY_ENABLE_LEDCTL=1/ZFS_PAY_ENABLE_LEDCTL=1/' /etc/default/zfs-pay
systemctl start zfs-pay-reconcile.service
```

## Removal

Normal removal first attempts to switch off only locate LEDs recorded as managed by zfs-pay. A cleanup failure is reported but does not block package removal. Configuration and cached mapping state are retained for reinstall:

```sh
apt remove zfs-pay
```

Purge removes the retained configuration and `/var/lib/zfs-pay` state:

```sh
apt purge zfs-pay
```

Neither operation changes ZFS pool membership, vdev state, rebuild state, firmware, or RAID configuration.

## Diagnostics

```sh
zfs-pay status
zfs-pay status --json
systemctl status zfs-pay-reconcile.service
journalctl -u zfs-pay-reconcile.service
```

The package does not require a controller utility at install time. A missing `zpool`, insufficient permissions, unsupported enclosure, or missing controller utility is reported through the CLI without guessing a bay.
