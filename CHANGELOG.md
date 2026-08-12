# Changelog

All notable changes to this project will be documented in this file. The format follows Keep a Changelog, and release versions will follow Semantic Versioning.

## [Unreleased]

### Added

- Read-only ZFS, Linux block-device, and physical bay discovery.
- StorCLI-compatible, Linux SES, and opt-in ledctl locate backends.
- Compact status/JSON output and exact-target locate with dry-run and timed auto-off.
- ZED event automation with verified missing-disk cache, file locking, deduplication, and managed LED ownership.
- Reproducible `amd64` and `arm64` Debian packages with safe install, upgrade, remove, and purge behavior.
- Unit, race, cross-module fixture, package lifecycle, release audit, and CI quality gates.

### Security

- Controller actions are restricted to fixed locate on/off commands and ambiguous targets fail closed.
- Proprietary controller utilities, credentials, and real inventory are excluded from release artifacts.
