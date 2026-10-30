# Community testing candidate

Independent OpenWrt integration for armanlife/firefox-vpn-client core 1.0.5.
Documentation and installer messages: Persian and US English.

Choose the correct Linux CPU archive and its matching `.sha256` file. Extract
on the router and run `sh install.sh --check` first. Other platforms require
`--experimental`. See the included installation and compatibility guides.

**Application archives only: do not flash them as firmware.** All archives include
corresponding core source and GPL license. Mozilla / Firefox account required.

Runtime components were reported working on the original Google WiFi reference
installation, including after reboot. This new installer and all other device or
firmware combinations still need real-device testing. Successful builds do not
establish device compatibility. Keep working legacy test1 installations unchanged.

فارسی: این نسخه برای تست جامعه است. آرشیو فریم‌ور نیست. ابتدا معماری و checksum را کنترل و بررسی بدون نصب را اجرا کنید. نصب‌کننده جدید روی روتر تأیید نشده است؛ نتیجه تست را بدون اطلاعات خصوصی در فرم گزارش ثبت کنید.
