# Device test checklist

[English](TESTING.md) · [فارسی](TESTING.fa.md) · [Русский](TESTING.ru.md) · [العربية](TESTING.ar.md)

Use a spare device or a maintenance window and keep a working management path.
Do not replace a working legacy test1 installation. Preserve account data locally.

Record each item as **pass / fail / not tested**. Do not collapse the list into
“works” if some items were skipped.

1. Exact model, hardware revision, CPU, firmware build and package version recorded.
2. Archive checksum verified; correct CPU archive selected.
3. `install.sh --check` passes (with `--experimental` if required).
4. Fresh install completes; report separately if this is a reinstall/upgrade.
5. Service starts and Services → Firefox VPN opens using LAN IPv4 management.
6. Mozilla account login and Test connection succeed.
7. PassWall2 Localhost Proxy is off; the LAN client has no alternate VPN or data path.
8. LAN browsing works through the selected node; record DNS failures separately.
9. After idle/sleep, a new request works again.
10. Service restart preserves account data and restores browsing.
11. Reboot restores service and LAN browsing without manually starting the core.
12. Optional reinstall preserves account data. Optional uninstall removes the
    service/menu and preserves accounts; select another node before uninstalling.

Do not include exact public IP, hostname, Wi-Fi name, MAC, serial, email, password,
tokens, verification codes, account backups or full logs. The local diagnostics
script has an allowlist; review its output before sharing. No telemetry is collected.

[Submit a report](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose).
A maintainer reviews evidence before changing compatibility status. One successful
report applies to that exact combination; it does not certify an entire brand.

