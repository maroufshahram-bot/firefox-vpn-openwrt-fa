#!/bin/sh
# OpenWrt Google WiFi installer. Does not modify PassWall2 or firewall rules.
set -eu
umask 077
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fail() { echo "خطا: $*" >&2; exit 1; }
[ "$(id -u)" = 0 ] || fail 'با کاربر root اجرا کنید.'
[ -f /etc/openwrt_release ] || fail 'این نصب‌کننده مخصوص OpenWrt است.'
. /etc/openwrt_release
[ "${DISTRIB_RELEASE:-}" = 25.12.1 ] || fail 'فقط نسخه 25.12.1 در دامنه این بسته است.'
[ "${DISTRIB_TARGET:-}" = ipq40xx/chromium ] || fail 'پلتفرم متفاوت است.'
[ "$(uname -m)" = armv7l ] || fail 'معماری باید armv7l باشد.'
model=$(ubus call system board | jsonfilter -e '@.model')
[ "$model" = 'Google WiFi (Gale)' ] || fail "مدل آزمایش‌شده Google WiFi (Gale) است؛ مدل شما: $model"
if [ -f /root/firefox-openwrt-armv7-test1/data/config.json ] && [ ! -f /etc/firefox-vpn/config.json ]; then
 fail 'نصب آزمایشی قبلی پیدا شد؛ برای حفظ حساب‌ها، مهاجرت لازم است. نصب متوقف شد.'
fi
[ -f /www/luci-static/resources/view.js ] || fail 'LuCI نصب نیست.'
command -v sha256sum >/dev/null || fail 'sha256sum موجود نیست.'
(cd "$base" && sha256sum -c SHA256SUMS) || fail 'صحت فایل اجرایی تأیید نشد.'
lan=$(ubus call network.interface.lan status | jsonfilter -e '@["ipv4-address"][0].address')
[ -n "$lan" ] || fail 'آدرس IPv4 شبکه lan پیدا نشد.'
free_kb=$(df -Pk /usr | awk 'END {print $4}')
[ "$free_kb" -gt 40000 ] || fail 'حداقل 40 مگابایت فضای آزاد لازم است.'
mkdir -p /usr/lib/firefox-vpn /etc/firefox-vpn
chmod 700 /etc/firefox-vpn
# Existing accounts/configuration are preserved. Stop only our installed service.
if [ -x /etc/init.d/firefox-vpn ]; then /etc/init.d/firefox-vpn stop; fi
cp "$base/bin/firefox-vpn-core" /usr/lib/firefox-vpn/firefox-vpn-core.new
chmod 755 /usr/lib/firefox-vpn/firefox-vpn-core.new
mv /usr/lib/firefox-vpn/firefox-vpn-core.new /usr/lib/firefox-vpn/firefox-vpn-core
if [ ! -f /etc/firefox-vpn/config.json ]; then
  /usr/lib/firefox-vpn/firefox-vpn-core -setup-server admin -data-dir /etc/firefox-vpn > /etc/firefox-vpn/initial-login.txt
  sed -i "s/0\.0\.0\.0:10820/$lan:10820/g; s/0\.0\.0\.0:10810/127.0.0.1:10810/g" /etc/firefox-vpn/config.json
fi
# Store daily logs in RAM; account/config files remain persistent.
if [ ! -e /etc/firefox-vpn/logs ] && [ ! -L /etc/firefox-vpn/logs ]; then
  ln -s /tmp/firefox-vpn-logs /etc/firefox-vpn/logs
fi
cat > /etc/init.d/firefox-vpn <<'SERVICE'
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1
start_service() {
    mkdir -p /tmp/firefox-vpn-logs
    chmod 700 /tmp/firefox-vpn-logs
    procd_open_instance
    procd_set_param command /usr/lib/firefox-vpn/firefox-vpn-core -plain -data-dir /etc/firefox-vpn
    procd_set_param respawn 3600 5 5
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
SERVICE
chmod 755 /etc/init.d/firefox-vpn
mkdir -p /usr/share/luci/menu.d /www/luci-static/resources/view
cat > /usr/share/luci/menu.d/luci-app-firefox-vpn.json <<'MENU'
{"admin/services/firefox-vpn":{"title":"Firefox VPN — فایرفاکس","order":90,"action":{"type":"view","path":"firefox-vpn"}}}
MENU
cat > /www/luci-static/resources/view/firefox-vpn.js <<'VIEW'
'use strict';
'require view';
return view.extend({
    render: function() {
        var url = 'http://' + window.location.hostname + ':10820/app/';
        window.location.replace(url);
        return E('a', {href: url}, 'بازکردن پنل فایرفاکس');
    },
    handleSaveApply: null, handleSave: null, handleReset: null
});
VIEW
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache
/etc/init.d/firefox-vpn enable
/etc/init.d/firefox-vpn start
printf '\nنصب انجام شد. پنل: http://%s:10820/app/\n' "$lan"
if [ -f /etc/firefox-vpn/initial-login.txt ]; then
 echo 'مشخصات اولیه ورود؛ خصوصی نگه دارید:'
 cat /etc/firefox-vpn/initial-login.txt
fi
echo 'قبل از انتخاب نود فایرفاکس، Localhost Proxy را در PassWall2 خاموش کنید.'
echo 'راهنمای README.md را برای افزودن حساب و نود دنبال کنید.'
