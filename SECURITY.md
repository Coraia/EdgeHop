# Security Policy

## Supported versions

Only the latest EdgeHop release receives security fixes.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting for this repository:

https://github.com/Coraia/EdgeHop/security/advisories/new

Do not include pairing keys, private logs, IP addresses, or other credentials in
public issues. We will acknowledge a valid report as soon as practical and
coordinate disclosure after a fix is available.

## Security boundaries

EdgeHop intentionally requires:

- macOS Accessibility permission to capture and suppress local input.
- Linux access to `/dev/uinput` to create the virtual input device.
- Network access between the paired devices on TCP port `24800`.

The project does not operate a cloud relay and does not collect telemetry.
