# Release process

No release step commits, tags, pushes, uploads, or publishes automatically. Remote operations require explicit maintainer authorization.

## Candidate checklist

1. Start from a reviewed clean commit and choose a Debian-compatible version.
2. Run `make verify` and `go test -race ./...`.
3. Run `make build-linux` and inspect both ELF architectures.
4. Run `make package PACKAGE_VERSION=<version>` twice and compare `SHA256SUMS`.
5. Run `make package-test` for install, upgrade, remove, purge, service, hook, managed LED, and residual-file checks.
6. Run `./tests/integration/release-audit.sh` to create `dist/release/` and audit package contents, credentials, proprietary binaries, license, SBOM, and third-party boundaries.
7. With fresh explicit authorization, run the real PVE smoke test documented below.
8. Review `CHANGELOG.md`, compatibility claims, and all generated checksums before any tag or upload.

## Controlled PVE smoke test

The test is intentionally non-destructive:

1. Record `zpool status -x` and confirm all expected pools are healthy or explain any pre-existing degradation.
2. Inspect the package with `dpkg-deb -I` and `dpkg-deb -c`, then install it.
3. Run `zfs-pay status --json` and retain only anonymized conclusions.
4. Select one explicitly confirmed healthy disk and run `locate --dry-run`.
5. Run a short timed locate, confirm the physical light, and confirm automatic off.
6. Run an explicit `--off` and repeat `zpool status -x`.
7. Confirm no VM, ZFS pool, vdev state, or controller configuration changed.
8. Verify the service and ZED hook paths; do not inject faults, offline disks, detach devices, or pull drives.

If any target is ambiguous or the pre-test pool state is unexpected, stop before LED mutation.

## Artifacts

The local candidate directory contains the two `.deb` files, `SHA256SUMS`, `SBOM.spdx.json`, and `THIRD_PARTY.md`. Proprietary controller utilities are runtime prerequisites supplied separately and must never be uploaded with the release.
