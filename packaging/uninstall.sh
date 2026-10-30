#!/bin/sh
set -eu
lang=${1:-en}
case "$lang" in en|fa|ru|ar) ;; *) echo 'Usage: sh uninstall.sh [en|fa|ru|ar]' >&2; exit 2 ;; esac
[ "$(id -u)" = 0 ] || exit 1
if [ -x /etc/init.d/firefox-vpn ]; then
    /etc/init.d/firefox-vpn stop
    /etc/init.d/firefox-vpn disable
fi
rm -f /etc/init.d/firefox-vpn /usr/lib/firefox-vpn/firefox-vpn-core /usr/lib/firefox-vpn/PACKAGE_VERSION /usr/lib/firefox-vpn/diagnose.sh
rm -f /usr/share/luci/menu.d/luci-app-firefox-vpn.json /www/luci-static/resources/view/firefox-vpn.js
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache
case "$lang" in
    fa) echo 'برنامه حذف شد؛ حساب‌ها و تنظیمات در /etc/firefox-vpn حفظ شدند.' ;;
    ru) echo 'Удалено. Аккаунты и настройки в /etc/firefox-vpn сохранены.' ;;
    ar) echo 'تمت الإزالة. حُفظت الحسابات والإعدادات في /etc/firefox-vpn.' ;;
    *) echo 'Uninstalled. Accounts and configuration in /etc/firefox-vpn were preserved.' ;;
esac
