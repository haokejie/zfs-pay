# Compatibility

This matrix separates automated evidence from real hardware evidence. A fixture pass demonstrates parser and safety behavior, not universal hardware compatibility.

## Platforms

| Platform | Architecture | Evidence | Status |
| --- | --- | --- | --- |
| Debian 12 container | amd64 | Buildx package, install, upgrade, remove, purge, systemd unit verification | Verified in isolation |
| Debian package build | arm64 | Cross-compiled static binaries, package content and architecture audit | Verified build artifact |
| Proxmox VE 9.1.1 / OpenZFS 2.3.4 | amd64 | Package install, three ONLINE disks, StorCLI mapping, timed and sustained locate/off, startup reconcile, visual LED confirmation and unchanged `zpool status -x` | Verified on physical hardware |

## Discovery and LED backends

| Component | Evidence | Status |
| --- | --- | --- |
| OpenZFS `zpool status` with GUID and `upath` | Anonymous multi-pool, mirror, log, spare and unavailable-vdev fixtures | Automated |
| GUID/path fallback snapshots | Topology consistency and malformed-output tests | Automated |
| `lsblk` and udev identity | Partition-to-parent, serial, WWN, missing identity and partial diagnostics | Automated |
| StorCLI-compatible JSON | Multiple controllers/enclosures, duplicate serial, WWN disambiguation and strict locate allowlist | Automated |
| Linux SES sysfs | Temporary sysfs tree, exact link, locate read-back and unsupported hardware behavior | Automated |
| ledctl | Disabled-by-default policy and exact `locate`/`locate_off` argument tests | Automated only |
| ZED automation | ONLINE, FAULTED, UNAVAIL, cached missing disk, duplicate event, lock and managed LED cleanup | Automated |

## Known boundaries

- StorCLI and PERCCLI are external proprietary dependencies and are not packaged.
- Hardware without a stable serial/WWN mapping is reported as unsupported or ambiguous.
- SES support requires the Linux kernel to expose an exact block-device enclosure link and a working `locate` attribute.
- `ledctl` remains off until the administrator validates it for the specific controller and backplane.
- PVE package paths, ZED daemon restart, hook syntax, startup reconcile, journald output and physical locate LED feedback were verified. A real ZED fault event was intentionally not injected; event-state behavior remains covered by anonymous automated tests.
