# Firefox VPN for OpenWrt

**An independent OpenWrt integration for the unofficial Firefox VPN client.**

[English](README.md) · [فارسی](README.fa.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [Install](docs/INSTALL.md) · [Compatibility](docs/COMPATIBILITY.md) · [Report a test](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose)

[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Validation](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml/badge.svg)](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml)

Run the [upstream core](https://github.com/armanlife/firefox-vpn-client) as an
OpenWrt service, open its dashboard from LuCI, and use its authenticated local
SOCKS proxy with PassWall2. No desktop environment or Firefox browser is required
on the router. A **Mozilla / Firefox account is required**.

This project is not affiliated with or endorsed by Mozilla. Account eligibility,
service availability, speed, quotas, and future access are controlled upstream.

## Start here

1. Read the [compatibility matrix](docs/COMPATIBILITY.md) before downloading.
2. Follow the [installation guide](docs/INSTALL.md), including the PassWall2 loop-prevention step.
3. Run the [device test checklist](docs/TESTING.md) and share the result.

**Current status: release candidate.** Runtime components have been reported
working on one Google WiFi (Gale) installation, including after reboot. The new
integrated installer and every other device/firmware combination still require
real-device testing. A passing build is not a device compatibility claim.

## What is included

| Component | Purpose |
| --- | --- |
| Preflight checks | Validate firmware family, CPU, executable, LuCI, and available storage before installing |
| OpenWrt service | Start at boot and restart the core after unexpected exits through procd |
| LuCI menu entry | Open the upstream dashboard from Services → Firefox VPN |
| Persistent account storage | Preserve configuration and accounts during reinstall and uninstall |
| Local diagnostics | Generate a small allowlisted report without uploading it |
| Community build pipeline | Build ARMv7, ARM64, x86-64, MIPS and MIPS little-endian archives |

Documentation and installer messages are available in Persian, US English, Russian and Arabic.
The v0.2.0-rc.2 install archives include all four installer languages.
The upstream dashboard is not translated by this project.

## Downloads and experimental builds

[Community testing candidate v0.2.0-rc.2](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/tag/v0.2.0-rc.2)
is available. All five CPU archives passed cross-compilation and packaging checks.

| CPU target | Install archive | Integrity check |
| --- | --- | --- |
| armv7 | [Download](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz.sha256) |
| arm64 | [Download](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz.sha256) |
| amd64 | [Download](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz.sha256) |
| mips | [Download](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz.sha256) |
| mipsle | [Download](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz.sha256) |

**Build success is not a device test.** Archives are application installers,
not firmware images, backups, `.ipk`, or `.apk` packages. Choose the CPU target
from the [compatibility guide](docs/COMPATIBILITY.md). Other platforms require
explicit `--experimental` opt-in; no brand-wide support is implied.

Development artifacts are available from successful [build workflow runs](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/build.yml).
Check the actual run result before downloading.

## Limits to understand

- TCP proxying only; SOCKS5 UDP is not supported. Some calls and games will not work.
- The pinned upstream version documents destination ports 80, 443, 8080, and 8443.
- The dashboard uses HTTP on the router's LAN IPv4 address. Use a trusted LAN;
  do not expose it to WAN. Existing configuration is preserved, so review its bindings.
- PassWall2 is configured separately. Turn off Localhost Proxy before selecting
  this node to prevent the core from proxying its own connection back into itself.
- Legacy `test1` installations require a separate migration; do not replace a
  working installation with this release candidate.

## Help improve compatibility

[Report a successful or failed device test](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose).
Include the exact model, firmware version, architecture, package version and
individual test outcomes. Reports in Persian, English, Russian and Arabic are welcome.

See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md),
[Changelog](CHANGELOG.md), and [build instructions](docs/BUILDING.md).

## Credits and license

Created and maintained by **Shahram Marouf**. The core is developed by
[armanlife/firefox-vpn-client](https://github.com/armanlife/firefox-vpn-client);
this repository supplies the OpenWrt integration, packaging and documentation.
See [upstream provenance](src/UPSTREAM.md). Distributed under [GPL-3.0](LICENSE).

