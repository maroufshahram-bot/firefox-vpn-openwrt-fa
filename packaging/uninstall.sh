#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
if [ -x /etc/init.d/firefox-vpn ]; then
 /etc/init.d/firefox-vpn stop
 /etc/init.d/firefox-vpn disable
fi
rm -f /etc/init.d/firefox-vpn /usr/lib/firefox-vpn/firefox-vpn-core
rm -f /usr/share/luci/menu.d/luci-app-firefox-vpn.json /www/luci-static/resources/view/firefox-vpn.js
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache
echo 'برنامه حذف شد؛ حساب‌ها و تنظیمات در /etc/firefox-vpn حفظ شدند.'
echo 'نود دیگری در PassWall2 انتخاب کنید.'
