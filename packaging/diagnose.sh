#!/bin/sh
# Deliberate allowlist: no addresses, account files, logs or hostnames.
set -eu
printf 'Report schema: 1\n'
printf 'CPU architecture: %s\n' "$(uname -m)"
if [ -f /etc/openwrt_release ]; then
    sed -n '/^DISTRIB_ID=/p; /^DISTRIB_RELEASE=/p; /^DISTRIB_TARGET=/p' /etc/openwrt_release
fi
if command -v ubus >/dev/null && command -v jsonfilter >/dev/null; then
    model=$(ubus call system board 2>/dev/null | jsonfilter -e '@.model' || true)
    printf 'Model: %s\n' "$model"
fi
if command -v apk >/dev/null; then
    printf 'Package manager: apk\n'
elif command -v opkg >/dev/null; then
    printf 'Package manager: opkg\n'
else
    printf 'Package manager: unknown\n'
fi
if [ -f /usr/lib/firefox-vpn/PACKAGE_VERSION ]; then
    printf 'Package version: '
    cat /usr/lib/firefox-vpn/PACKAGE_VERSION
fi
if [ -x /etc/init.d/firefox-vpn ]; then
    if /etc/init.d/firefox-vpn running >/dev/null 2>&1; then
        printf 'Service: running\n'
    else
        printf 'Service: stopped\n'
    fi
else
    printf 'Service: not installed\n'
fi
printf 'Review this report before sharing. No report has been uploaded.\n'
