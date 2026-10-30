# Compatibility

[فارسی](COMPATIBILITY.fa.md)

Compatibility belongs to a **model + hardware revision + firmware + package**
combination. The same brand or CPU architecture does not establish compatibility.

## Evidence currently available

| Device / environment | Evidence | Installer status |
| --- | --- | --- |
| Google WiFi (Gale), ARMv7, ipq40xx/chromium, OpenWrt 25.12.1 r32768-b21cfa8f8c, kernel 6.12.74 | Owner reports successful account login, core connection test, LuCI shortcut, LAN browsing through PassWall2, and operation after reboot with core 1.0.5 | New integrated installer not device-tested |
| Any other model, hardware revision or firmware | No verified device report in this repository | Experimental; explicit opt-in required |

The installer recognizes the reference model, release, target and CPU; that
recognition is not a claim that this release candidate has passed device tests.
No community reports have yet been promoted to verified compatibility.

## CPU archives

| Archive suffix | Expected CPU | Notes |
| --- | --- | --- |
| linux-armv7 | 32-bit ARMv7, `armv7l` | Original Google WiFi reference architecture |
| linux-arm64 | 64-bit ARM, `aarch64` | Experimental |
| linux-amd64 | x86-64, `x86_64` | Experimental; x86 OpenWrt machines |
| linux-mips | 32-bit MIPS, big-endian | Experimental, software floating point |
| linux-mipsle | 32-bit MIPS, little-endian | Experimental, software floating point |

The build workflow is a requested target matrix, not a claim that all outputs
have already built. Check the actual run before downloading. Unsupported CPUs
(ARMv5/v6, x86 32-bit, MIPS64, etc.) are rejected. MIPS kernels may report `mips`
for either endianness; the installer also executes a local `-version` probe and
rejects a binary it cannot run. No network or account operation is part of this probe.

## Common router examples

These mappings help choose an archive on an **already installed, compatible
OpenWrt-family system**. They do not certify this package on those devices.
Architecture references were checked against OpenWrt device data on 2026-10-30.

| Exact model | Candidate archive | Evidence and restriction |
| --- | --- | --- |
| GL.iNet GL-MT6000 (Flint 2) | linux-arm64 | [Official device data](https://openwrt.org/toh/hwdata/gl.inet/gl.inet_gl-mt6000); package device-test pending |
| GL.iNet GL-MT3000 (Beryl AX) | linux-arm64 | [Official device data](https://openwrt.org/toh/hwdata/gl.inet/gl.inet_gl-mt3000); package device-test pending |
| Xiaomi AX3000T RD03 / RD23, MediaTek variants | linux-arm64 | [Official device data](https://openwrt.org/toh/hwdata/xiaomi/xiaomi_mi_router_ax3000t) and [hardware differences](https://openwrt.org/toh/xiaomi/ax3000t); excludes RD03v2; package device-test pending |

Run preflight on the actual system. A stock vendor OS, changed hardware revision,
or missing storage/dependencies can invalidate an otherwise correct CPU choice.

## Firmware scope

| Family | Initial scope | Conditions |
| --- | --- | --- |
| OpenWrt | Reference platform plus experimental other combinations | LuCI, procd, ubus, jsonfilter, IPv4 LAN and sufficient storage |
| ImmortalWrt | Experimental | Same services; no device validation yet |
| GL.iNet stock firmware | Experimental candidate | Must report an accepted OpenWrt family and provide the required services; model-specific differences remain untested |
| Asuswrt-Merlin, DD-WRT, RouterOS and vendor stock firmware outside this family | Not supported by this installer | Need a separate service/integration design; do not force installation |

## Community testing priorities

Prioritize exact OpenWrt-supported model/revision combinations among GL.iNet,
GL.iNet Flint/Slate families, Xiaomi/Redmi, TP-Link and Cudy, followed by x86-64
OpenWrt systems. These are **recruitment targets, not supported-device claims**.
Do not flash a firmware image based on this list. Verify firmware support with
the firmware project's official device documentation first.

For each report, record the exact model/revision, firmware build, CPU, package,
preflight result, fresh-install/upgrade status, login, LAN browsing, sleep/wake,
service restart and reboot. Use [the test checklist](TESTING.md).
Maintainer review is required before updating this evidence table.
