#!/bin/sh
# OpenWrt-family integration. Never changes firewall or PassWall2 settings.
set -eu
umask 077
base=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
lang=en
experimental=0
check_only=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --lang) [ "$#" -ge 2 ] || exit 2; lang=$2; shift ;;
        --experimental) experimental=1 ;;
        --check) check_only=1 ;;
        --help|-h)
            echo 'Usage: sh install.sh [--lang en|fa|ru|ar] [--check] [--experimental]'
            echo 'Experimental installs are not verified on your router.'
            exit 0 ;;
        *) echo "Unknown argument: $1" >&2; exit 2 ;;
    esac
    shift
done
case "$lang" in en|fa|ru|ar) ;; *) echo 'Language must be en, fa, ru or ar.' >&2; exit 2 ;; esac
say() {
    case "$lang" in
        fa) printf '%s\n' "$2" ;;
        ru) printf '%s\n' "$3" ;;
        ar) printf '%s\n' "$4" ;;
        *) printf '%s\n' "$1" ;;
    esac
}
fail() { say "$@" >&2; exit 1; }
[ "$(id -u)" = 0 ] || fail 'Run as root.' 'با کاربر root اجرا کنید.' 'Запустите от root.' 'شغّل بصلاحية root.'
[ -f /etc/openwrt_release ] || fail 'OpenWrt-family firmware is required.' 'فریم‌ور خانواده OpenWrt لازم است.' 'Нужна прошивка семейства OpenWrt.' 'يلزم برنامج ثابت من عائلة OpenWrt.'
# shellcheck disable=SC1091
. /etc/openwrt_release
for tool in ubus jsonfilter sha256sum awk df sed mktemp cp mv; do
    command -v "$tool" >/dev/null || fail "Missing dependency: $tool" "پیش‌نیاز موجود نیست: $tool" "Отсутствует зависимость: $tool" "متطلب غير موجود: $tool"
done
[ -f "$base/platform.sh" ] || fail 'Incomplete package.' 'بسته ناقص است.' 'Неполный пакет.' 'الحزمة غير مكتملة.'
# shellcheck disable=SC1091
. "$base/platform.sh"
ffvpn_supported_family "${DISTRIB_ID:-}" || fail 'Unsupported firmware family.' 'این خانواده فریم‌ور پشتیبانی نمی‌شود.' 'Семейство прошивки не поддерживается.' 'عائلة البرنامج الثابت غير مدعومة.'
[ -f /etc/rc.common ] && [ -x /sbin/procd ] || fail 'procd service management is required.' 'مدیریت سرویس procd لازم است.' 'Требуется управление службами procd.' 'تلزم إدارة الخدمات عبر procd.'
[ -f /www/luci-static/resources/view.js ] || fail 'LuCI is required.' 'LuCI نصب نیست.' 'Требуется LuCI.' 'يلزم LuCI.'
model=$(ubus call system board | jsonfilter -e '@.model')
machine=$(uname -m)
[ -f "$base/TARGET" ] && [ -f "$base/PACKAGE_VERSION" ] || fail 'Package metadata is missing.' 'مشخصات بسته موجود نیست.' 'Нет метаданных пакета.' 'بيانات الحزمة الوصفية غير موجودة.'
target=$(cat "$base/TARGET")
ffvpn_arch_matches "$target" "$machine" || fail 'Wrong CPU package. See the compatibility guide.' 'بسته با معماری پردازنده سازگار نیست؛ راهنمای سازگاری را ببینید.' 'Неверный пакет CPU. См. руководство по совместимости.' 'حزمة المعالج غير مناسبة؛ راجع دليل التوافق.'
if ! ffvpn_reference_platform "${DISTRIB_ID:-}" "${DISTRIB_RELEASE:-}" "${DISTRIB_TARGET:-}" "$model" "$machine"; then
    [ "$experimental" = 1 ] || fail 'Untested platform. Review docs/COMPATIBILITY.md; use --experimental to opt in.' 'این محیط تست نشده است؛ راهنمای سازگاری را بخوانید و برای پذیرش نصب آزمایشی از --experimental استفاده کنید.' 'Платформа не проверена. Прочитайте docs/COMPATIBILITY.ru.md; для согласия используйте --experimental.' 'المنصة غير مختبرة. راجع docs/COMPATIBILITY.ar.md واستخدم --experimental للقبول.'
    say 'EXPERIMENTAL: no device compatibility has been verified.' 'آزمایشی: سازگاری روی این دستگاه تأیید نشده است.' 'ЭКСПЕРИМЕНТАЛЬНО: совместимость устройства не подтверждена.' 'تجريبي: لم يُتحقق من توافق الجهاز.'
fi
if [ -f /root/firefox-openwrt-armv7-test1/data/config.json ] && [ ! -f /etc/firefox-vpn/config.json ]; then
    fail 'Legacy test1 data found. Automatic migration is not available; keep the working installation.' 'داده نسخه test1 پیدا شد؛ مهاجرت خودکار نداریم. نصب کارای قبلی را حفظ کنید.' 'Найдены данные старой test1. Автопереноса нет; сохраните рабочую установку.' 'عُثر على بيانات test1 قديمة. لا يتوفر ترحيل تلقائي؛ احتفظ بالتثبيت العامل.'
fi
# Check only the expected binary, never a path supplied by checksum metadata.
expected=$(awk '$2 == "bin/firefox-vpn-core" {print $1}' "$base/SHA256SUMS")
actual=$(sha256sum "$base/bin/firefox-vpn-core" | awk '{print $1}')
[ -n "$expected" ] && [ "$actual" = "$expected" ] || fail 'Binary checksum mismatch.' 'صحت فایل اجرایی تأیید نشد.' 'Контрольная сумма исполняемого файла не совпадает.' 'بصمة الملف التنفيذي غير مطابقة.'
[ -x "$base/bin/firefox-vpn-core" ] || fail 'Binary is not executable; extract the archive on the router.' 'فایل اجرایی قابل اجرا نیست؛ آرشیو را روی خود روتر استخراج کنید.' 'Файл не исполняемый; распакуйте архив на роутере.' 'الملف غير قابل للتنفيذ؛ فك الأرشيف على الموجّه.'
"$base/bin/firefox-vpn-core" -version >/dev/null 2>&1 || fail 'The binary cannot run on this CPU/kernel.' 'فایل اجرایی روی این پردازنده یا کرنل اجرا نمی‌شود.' 'Файл не запускается на этом CPU/ядре.' 'لا يعمل الملف التنفيذي على هذا المعالج أو النواة.'
lan=$(ubus call network.interface.lan status | jsonfilter -e '@["ipv4-address"][0].address')
# Only an IPv4 LAN binding is implemented. Do not fall back to all interfaces.
case "$lan" in ''|*[!0-9.]*) fail 'A LAN IPv4 address is required.' 'آدرس IPv4 شبکه lan لازم است.' 'Требуется IPv4-адрес LAN.' 'يلزم عنوان LAN IPv4.' ;; esac
free_kb=$(df -Pk /usr | awk 'END {print $4}')
binary_kb=$(du -k "$base/bin/firefox-vpn-core" | awk '{print $1}')
needed_kb=$((binary_kb * 2 + 4096))
[ "$needed_kb" -ge 40000 ] || needed_kb=40000
[ "$free_kb" -gt "$needed_kb" ] || fail 'Insufficient persistent storage for installation and rollback.' 'فضای دائمی برای نصب و بازگردانی کافی نیست.' 'Недостаточно постоянного хранилища для установки и отката.' 'مساحة التخزين الدائمة غير كافية للتثبيت والاستعادة.'
if [ "$check_only" = 1 ]; then
    say 'Preflight passed. Nothing was installed. This is not a device or connectivity test.' 'بررسی پیش‌نیازها موفق بود؛ چیزی نصب نشد. این نتیجه به معنی تست عملکرد یا اتصال نیست.' 'Предварительная проверка пройдена. Ничего не установлено. Это не тест устройства или соединения.' 'نجح الفحص المسبق. لم يُثبّت شيء. هذا ليس اختباراً للجهاز أو الاتصال.'
    exit 0
fi
# Stage new configuration before stopping the running service.
stage=$(mktemp -d /tmp/firefox-vpn-install.XXXXXX)
changed=0
was_running=0
managed='/usr/lib/firefox-vpn/firefox-vpn-core /usr/lib/firefox-vpn/PACKAGE_VERSION /usr/lib/firefox-vpn/diagnose.sh /etc/init.d/firefox-vpn /usr/share/luci/menu.d/luci-app-firefox-vpn.json /www/luci-static/resources/view/firefox-vpn.js'
cleanup() {
    result=$?
    trap - EXIT HUP INT TERM
    if [ "$result" -ne 0 ] && [ "$changed" = 1 ]; then
        /etc/init.d/firefox-vpn stop >/dev/null 2>&1 || true
        for path in $managed; do
            if [ -f "$stage/backup$path" ]; then
                cp -p "$stage/backup$path" "$path" || true
            else
                rm -f "$path"
            fi
        done
        if [ "$was_running" = 1 ]; then /etc/init.d/firefox-vpn start >/dev/null 2>&1 || true; fi
        say 'Install failed; previous managed files were restored. Account data was preserved.' 'نصب ناموفق بود؛ فایل‌های قبلی برنامه بازگردانده شدند و داده حساب‌ها حفظ شد.' 'Установка не удалась; прежние управляемые файлы восстановлены. Аккаунты сохранены.' 'فشل التثبيت؛ استُعيدت ملفات البرنامج السابقة وحُفظت الحسابات.' >&2
    fi
    rm -rf "$stage"
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
if [ ! -f /etc/firefox-vpn/config.json ]; then
    mkdir -p "$stage/data"
    "$base/bin/firefox-vpn-core" -setup-server admin -data-dir "$stage/data" > "$stage/data/initial-login.txt"
    sed -i "s/0\.0\.0\.0:10820/$lan:10820/g; s/0\.0\.0\.0:10810/127.0.0.1:10810/g" "$stage/data/config.json"
    [ "$(jsonfilter -i "$stage/data/config.json" -e '@.dashboard.listen')" = "$lan:10820" ] || fail 'Dashboard binding validation failed.' 'کنترل آدرس گوش‌دادن پنل ناموفق بود.' 'Проверка привязки панели не пройдена.' 'فشل التحقق من عنوان استماع اللوحة.'
    [ "$(jsonfilter -i "$stage/data/config.json" -e '@.proxy.listen')" = '127.0.0.1:10810' ] || fail 'Proxy binding validation failed.' 'کنترل آدرس گوش‌دادن پراکسی ناموفق بود.' 'Проверка привязки прокси не пройдена.' 'فشل التحقق من عنوان استماع الوكيل.'
fi
for path in $managed; do
    if [ -f "$path" ]; then
        mkdir -p "$stage/backup$(dirname "$path")"
        cp -p "$path" "$stage/backup$path"
    fi
done
if [ -x /etc/init.d/firefox-vpn ]; then
    if /etc/init.d/firefox-vpn running >/dev/null 2>&1; then was_running=1; fi
fi
changed=1
if [ -x /etc/init.d/firefox-vpn ]; then /etc/init.d/firefox-vpn stop; fi
mkdir -p /etc/init.d /usr/lib/firefox-vpn /etc/firefox-vpn /usr/share/luci/menu.d /www/luci-static/resources/view
chmod 700 /etc/firefox-vpn
cp "$base/bin/firefox-vpn-core" /usr/lib/firefox-vpn/firefox-vpn-core.new
chmod 755 /usr/lib/firefox-vpn/firefox-vpn-core.new
mv /usr/lib/firefox-vpn/firefox-vpn-core.new /usr/lib/firefox-vpn/firefox-vpn-core
cp "$base/PACKAGE_VERSION" "$base/diagnose.sh" /usr/lib/firefox-vpn/
chmod 755 /usr/lib/firefox-vpn/diagnose.sh
if [ -d "$stage/data" ]; then cp -R "$stage/data/." /etc/firefox-vpn/; fi
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
cat > /usr/share/luci/menu.d/luci-app-firefox-vpn.json <<'MENU'
{"admin/services/firefox-vpn":{"title":"Firefox VPN","order":90,"action":{"type":"view","path":"firefox-vpn"}}}
MENU
cat > /www/luci-static/resources/view/firefox-vpn.js <<'VIEW'
'use strict';
'require view';
return view.extend({
    render: function() {
        var host = window.location.hostname;
        if (host.indexOf(':') !== -1) host = '[' + host.replace(/^\[|\]$/g, '') + ']';
        var url = 'http://' + host + ':10820/app/';
        window.location.replace(url);
        return E('a', {href: url}, 'Open Firefox VPN / بازکردن پنل / Открыть Firefox VPN / فتح Firefox VPN');
    },
    handleSaveApply: null, handleSave: null, handleReset: null
});
VIEW
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache
/etc/init.d/firefox-vpn start
sleep 2
/etc/init.d/firefox-vpn running >/dev/null 2>&1 || fail 'Service did not start.' 'سرویس اجرا نشد.' 'Служба не запустилась.' 'لم تبدأ الخدمة.'
/etc/init.d/firefox-vpn enable
changed=0
say 'Installed. Open Services > Firefox VPN in LuCI using the LAN IPv4 address.' 'نصب انجام شد؛ با آدرس IPv4 شبکه داخلی، در LuCI به Services > Firefox VPN بروید.' 'Установлено. Откройте Services > Firefox VPN в LuCI через LAN IPv4.' 'تم التثبيت. افتح Services > Firefox VPN في LuCI عبر عنوان LAN IPv4.'
say 'Initial credentials, if newly generated, are in /etc/firefox-vpn/initial-login.txt. Keep them private.' 'مشخصات ورود تازه، در صورت ایجاد، در /etc/firefox-vpn/initial-login.txt است؛ خصوصی نگه دارید.' 'Новые начальные реквизиты находятся в /etc/firefox-vpn/initial-login.txt. Храните их в тайне.' 'بيانات الدخول الأولية، إن أُنشئت، في /etc/firefox-vpn/initial-login.txt. احتفظ بها سراً.'
say 'Disable Localhost Proxy in PassWall2 before selecting this node. Follow the installation guide.' 'پیش از انتخاب نود، Localhost Proxy را در PassWall2 خاموش کنید و راهنمای نصب را دنبال کنید.' 'Отключите Localhost Proxy в PassWall2 до выбора узла. Следуйте руководству.' 'عطّل Localhost Proxy في PassWall2 قبل اختيار العقدة واتبع دليل التثبيت.'

