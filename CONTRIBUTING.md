# Contributing

Contributions are welcome for additional anonymous fixtures, controller backends, diagnostics, documentation, and safety tests.

## Development

Use Go 1.24 and keep the default build free of CGO and third-party dependencies unless the change documents a clear need. Before opening a pull request, run:

```sh
make verify
go test -race ./...
make build-linux
```

Changes to packaging should also run `make package-test`. Parser and backend changes must include anonymous fixtures for malformed, missing, duplicate, and version-specific fields where applicable.

## Safety rules

- Never add disk offline, replace, rebuild, erase, firmware, or RAID configuration operations.
- Do not use `sh -c` or interpolate untrusted values into external commands.
- Keep locate targets exact and fail closed on missing or ambiguous identity.
- Do not commit server addresses, credentials, real serial-number inventories, raw customer logs, or proprietary binaries.
- Do not make a real hardware test destructive. Status and a user-confirmed locate/off are the maximum accepted scope.

Use focused commits following Conventional Commits. A pull request should describe the hardware or fixture evidence, commands run, and any remaining unsupported cases.
