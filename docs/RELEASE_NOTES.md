# v0.2.0-rc.2 — four-language community testing candidate

Independent OpenWrt integration for armanlife/firefox-vpn-client core 1.0.5.

## Changes since rc.1

- Add Russian and Arabic README, installation, compatibility and device-testing guides.
- Include US English, Persian, Russian and Arabic installer messages in every CPU archive.
- Select installer language with `--lang en`, `--lang fa`, `--lang ru` or `--lang ar`.
- Uninstall accepts `en`, `fa`, `ru` or `ar` as its first argument.
- Add language checks for preflight failures, install/uninstall and account-data preservation.
- Keep the upstream core and compatibility status unchanged. The upstream dashboard
  is not translated by this integration. Existing rc.1 assets are preserved.

## Installation

Choose the correct Linux CPU archive and its matching `.sha256` file. Extract
on the router and run `sh install.sh --lang en --check` first (choose your language).
Other platforms require `--experimental`. Read the included installation and
compatibility guides before installing.

**Application archives only: do not flash them as firmware.** All archives include
corresponding core source and GPL license. A Mozilla / Firefox account is required.

## Testing status

Runtime components were reported working on the original Google WiFi reference
installation, including after reboot. The integrated installer and all other device
or firmware combinations still need real-device testing. Successful automated tests
and CPU builds do not establish device compatibility. Keep working legacy test1
installations unchanged. Share test outcomes without private account or network data.

فارسی: نسخهٔ rc.2 راهنماها و پیام‌های نصب را به چهار زبان فارسی، انگلیسی، روسی و عربی ارائه می‌کند. ابتدا معماری، checksum و بررسی بدون نصب را کنترل کنید. این آرشیو فریم‌ور نیست و نصب‌کنندهٔ جدید هنوز روی روتر تأیید نشده است.

Русский: rc.2 включает руководства и сообщения установщика на четырёх языках.
Для русского используйте `--lang ru`. Сначала проверьте архив и выполните `--check`.
Это не образ прошивки; испытания установщика на реальных устройствах ещё требуются.

العربية: يتضمن rc.2 الأدلة ورسائل المثبّت بأربع لغات. استخدم `--lang ar` للعربية،
وتحقّق من الأرشيف ثم نفّذ `--check` أولاً. الأرشيف ليس برنامجاً ثابتاً؛ ما زال
المثبّت بحاجة إلى اختبار على أجهزة فعلية.
