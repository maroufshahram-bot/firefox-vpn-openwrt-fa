#!/bin/sh
set -eu
lang=${1:-en}
case "$lang" in en|fa) ;; *) echo 'Usage: sh uninstall.sh [en|fa]' >&2; exit 2 ;; esac
[ "$(id -u)" = 0 ] || exit 1
if [ -x /etc/init.d/firefox-vpn ]; then
    /etc/init.d/firefox-vpn stop
    /etc/init.d/firefox-vpn disable
fi
rm -f /etc/init.d/firefox-vpn /usr/lib/firefox-vpn/firefox-vpn-core /usr/lib/firefox-vpn/PACKAGE_VERSION /usr/lib/firefox-vpn/diagnose.sh
rm -f /usr/share/luci/menu.d/luci-app-firefox-vpn.json /www/luci-static/resources/view/firefox-vpn.js
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache
if [ "$lang" = fa ]; then
    echo 'برنامه حذف شد؛ حساب‌ها و تنظیمات در /etc/firefox-vpn حفظ شدند.'
else
    echo 'Uninstalled. Accounts and configuration in /etc/firefox-vpn were preserved.'
fi
