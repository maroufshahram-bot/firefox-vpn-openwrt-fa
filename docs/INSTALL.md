# Installation

[English](INSTALL.md) · [فارسی](INSTALL.fa.md) · [Русский](INSTALL.ru.md) · [العربية](INSTALL.ar.md) · [Compatibility](COMPATIBILITY.md)

The v0.2.0-rc.2 installer supports `--lang en|fa|ru|ar`. Use `en` for US English,
`fa` for Persian, `ru` for Russian or `ar` for Arabic. Uninstall accepts the same
language code as its first argument. Do not mix installer files from different builds.

## Before you start

- OpenWrt-family firmware, LuCI, procd, a supported CPU target, root access and
  at least 40 MB of free persistent storage (more may be required for larger binaries).
- A Mozilla / Firefox account, access to its email and any required verification.
- Working router Internet access. Account/server availability is not guaranteed.
- PassWall2 installed separately if you want to route LAN clients through the proxy.
- A way to restore your previous working node. The installer does not change
  firewall rules, network settings, DNS rules or PassWall2 configuration.

Do not install over a working legacy `test1` setup. Automatic migration is not provided.
The new installer has not yet been verified on a real router.

## 1. Connect to the router

Use SSH from a computer, phone or other device with a compatible client:

```sh
ssh root@ROUTER_ADDRESS
```

`ROUTER_ADDRESS` is a placeholder. Substitute your own router address locally;
do not publish it. A LuCI terminal is another option if already installed. Use a
trusted LAN or an existing secure management path; opening WAN ports is unnecessary.

## 2. Download, verify and transfer

Choose the correct CPU archive from a successful build or reviewed release.
Download its matching `.sha256` file. In the download directory on your computer:

```sh
sha256sum -c firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz.sha256
```

This example is for ARMv7. Change the filename for another target. A checksum
checks file integrity; it is not independent proof of publisher trust.

Upload the archive to `/root` with a LuCI file manager, WinSCP or SCP:

```sh
scp firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz root@ROUTER_ADDRESS:/root/
```

If OpenSSH SCP reports a missing SFTP subsystem, use `scp -O` or another transfer
method. Do not upload the archive to Flash Firmware, Restore Backup or the package manager.

## 3. Check before installing

On the router, extract the archive and run preflight:

```sh
cd /root
tar -xzf firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz
cd firefox-vpn-openwrt
sh install.sh --lang en --check
```

For a platform other than the exact reference model/release/target combination:

```sh
sh install.sh --lang en --experimental --check
```

`--check` does not install anything. A pass only establishes that the preflight
checks passed; it does not prove routing, login, reboot recovery or compatibility.
A firmware family outside OpenWrt/ImmortalWrt is rejected even with this flag.
GL.iNet builds must identify as an accepted OpenWrt family and supply the required
OpenWrt services; brand alone is insufficient.

## 4. Install

For the reference platform:

```sh
sh install.sh --lang en
```

For an explicitly accepted experimental platform:

```sh
sh install.sh --lang en --experimental
```

The installer preserves existing account data. New installations bind the
SOCKS proxy to `127.0.0.1:10810` and the HTTP dashboard to the LAN IPv4 address
on port `10820`. Existing configuration is not rewritten; review its bindings.
If installation fails after managed files are replaced, the installer attempts
to restore their previous copies. This is not a full router backup or migration.

Initial credentials are stored privately on the router. Read them locally:

```sh
cat /etc/firefox-vpn/initial-login.txt
```

Never post this output, account data or an unredacted terminal screenshot.
A fresh password is generated on first install; reinstalling preserves it.

## 5. Add an account

Refresh LuCI, then open **Services → Firefox VPN** while accessing LuCI using its
LAN IPv4 address. This opens the HTTP dashboard in the same tab. IPv6-only
management and reverse-proxy path access are not covered by this integration.

Log in using the dashboard credentials, then use **Accounts → Add account**.
Complete any email, two-factor or CAPTCHA checks. Select **Test connection**.
`Connection OK` indicates that test request succeeded, not a latency measurement.
Dashboard login, proxy login and Mozilla account login are separate credentials.

## 6. Configure PassWall2

**Before selecting the Firefox node, disable Localhost Proxy in PassWall2 → Basic
Settings and Save & Apply.** Otherwise the core's outbound connection can be
sent back into itself. Keep LAN client proxying enabled. This setting also affects
the router's own traffic when other nodes are selected; it is not restored automatically.

In **Node List → Add**:

| Field | Value |
| --- | --- |
| Remarks | Firefox VPN |
| Type | Xray |
| Protocol | Socks |
| Address | 127.0.0.1 |
| Port | 10810 |
| Username / Password | Proxy credentials, not dashboard or Mozilla login |
| TLS | Off |
| Transport | RAW (TCP) |
| Mux | Off |
| Chain Proxy | Close / Not use |

To read only the proxy credentials locally:

```sh
jsonfilter -i /etc/firefox-vpn/config.json -e '@.proxy.username' -e '@.proxy.password'
```

Do not share that output. Save the node, select it in Basic Settings and apply.
Test from a LAN client with its own VPN and alternate Internet paths disabled.
If browsing fails, return to the previously working node.

## Operation and troubleshooting

The service survives closing the terminal. `Sleeping` after inactivity can be
normal; a real request or Test connection should wake the tunnel. A running
service does not imply an active tunnel or working Internet access.

| Symptom | What to check |
| --- | --- |
| Loop warning or no Internet after selecting node | Disable Localhost Proxy and restart the core |
| Dashboard works; LAN browsing does not | Proxy credentials, PassWall2 LAN rules and DNS configuration |
| Sleeping | Trigger a real request before reporting failure |
| Could not fetch | Service state, LAN IPv4 access and dashboard HTTP URL |
| UDP refused | Upstream TCP-only limitation |
| LuCI menu missing | Refresh LuCI; check installation output and service state |

```sh
/etc/init.d/firefox-vpn status
/etc/init.d/firefox-vpn restart
sh /usr/lib/firefox-vpn/diagnose.sh
```

The diagnostics script prints selected platform fields only. It does not read
account/configuration files or upload anything. Review even this output before posting.
Keep full `logread` output private; share only a manually redacted relevant excerpt.

Follow the [test checklist](TESTING.md), including a reboot when convenient.
Do not claim a reboot test until browsing works again without manual service startup.

## Uninstall

Select another working PassWall2 node first, then from the extracted directory:

```sh
sh uninstall.sh en
```

Account data in `/etc/firefox-vpn` is preserved. No destructive account purge is
provided. This package does not implement automatic core updates; use reviewed
project releases, not the upstream desktop updater or a generic Linux installer.

