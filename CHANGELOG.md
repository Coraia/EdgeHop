# Changelog

## [0.3.0] - 2026-09-05

### Added

- EdgeHop product identity and macOS application icon.
- Menu-bar device layout selection for a client on the Mac's left or right.
- Runtime layout synchronization from the Mac to the Linux client.
- Automatic migration from the previous Universal Control installation.
- GitHub Actions validation and public release documentation.

### Changed

- Wire protocol version is now `edgehop/1`. v0.2.0 (universal_control) builds
  cannot connect to v0.3.0; upgrade both the Mac app and the Linux client.

### Security

- TLS 1.3 transport with channel-bound shared-secret authentication.
- Random 256-bit pairing secrets stored with private file permissions.
- Protocol frame size limits and strict typed payload validation.

### Fixed

- Connection ownership, disconnect cleanup, held input, and sticky-edge races.
- Side-specific modifiers, extended keys, and Caps Lock forwarding.
- Clipboard echo suppression and Hyprland/Wayland session recovery.
- Repeatable Omarchy upgrades and systemd pairing-key path handling.

[0.3.0]: https://github.com/Coraia/EdgeHop/releases/tag/v0.3.0
