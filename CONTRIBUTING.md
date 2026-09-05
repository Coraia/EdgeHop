# Contributing to EdgeHop

## Development

Requirements:

- Go 1.26
- macOS 26 and Xcode Command Line Tools for the host application
- Omarchy/Arch Linux with Hyprland for client integration testing

Before opening a pull request:

```bash
gofmt -w cmd internal tools
go test ./...
go vet ./...
bash -n scripts/*.sh
./scripts/setup-omarchy-test.sh
```

Keep platform behavior behind the existing `internal/server` and
`internal/client` seams. Add regression tests for protocol, input-state, layout,
installer, or migration changes.

Never commit pairing keys, machine-specific addresses, logs, or generated
contents of `bin/` and `dist/`.
