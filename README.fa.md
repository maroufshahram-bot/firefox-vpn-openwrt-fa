# فایرفاکس ⁦VPN⁩ برای ⁦OpenWrt⁩

**یکپارچه‌سازی مستقل کلاینت غیررسمی ⁦Firefox VPN⁩ با ⁦OpenWrt⁩**

[English](README.md) · [فارسی](README.fa.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [راهنمای نصب](docs/INSTALL.fa.md) · [سازگاری](docs/COMPATIBILITY.fa.md) · [ثبت نتیجه تست](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose)

[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Validation](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml/badge.svg)](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml)

این پروژه [هستهٔ اصلی](https://github.com/armanlife/firefox-vpn-client) را به‌صورت سرویس روی روتر اجرا می‌کند، دسترسی به پنل آن را از ⁦LuCI⁩ فراهم می‌کند و پراکسی محلی دارای احراز هویت را برای استفاده در ⁦PassWall2⁩ در اختیار می‌گذارد. نصب مرورگر فایرفاکس یا محیط دسکتاپ روی روتر لازم نیست. **حساب ⁦Mozilla / Firefox⁩ لازم است.**

این پروژه وابسته به موزیلا یا مورد تأیید رسمی آن نیست. شرایط حساب، دسترس‌پذیری سرویس، سرعت، سهمیه و دسترسی آینده تابع سرویس اصلی است.

## شروع

۱. پیش از دریافت فایل، [جدول سازگاری](docs/COMPATIBILITY.fa.md) را بخوانید.
۲. [راهنمای نصب](docs/INSTALL.fa.md)، به‌ویژه جلوگیری از حلقه در ⁦PassWall2⁩، را دنبال کنید.
۳. [چک‌لیست تست دستگاه](docs/TESTING.fa.md) را اجرا و نتیجه را ثبت کنید.

**وضعیت فعلی: نامزد انتشار.** عملکرد اجزای راهکار روی یک دستگاه ⁦Google WiFi (Gale)⁩، از جمله پس از ریبوت، گزارش شده است. نصب‌کنندهٔ یکپارچهٔ جدید و تمام ترکیب‌های دیگر دستگاه و فریم‌ور به تست واقعی نیاز دارند. ساخت موفق فایل، به معنی تأیید سازگاری دستگاه نیست.

## امکانات

| بخش | کاربرد |
| --- | --- |
| بررسی پیش‌نیازها | کنترل خانواده فریم‌ور، پردازنده، اجرای فایل، ⁦LuCI⁩ و فضای آزاد پیش از نصب |
| سرویس ⁦OpenWrt⁩ | اجرای خودکار پس از روشن‌شدن و تلاش برای اجرای مجدد با ⁦procd⁩ |
| منوی ⁦LuCI⁩ | دسترسی به پنل اصلی از ⁦Services → Firefox VPN⁩ |
| نگهداری حساب‌ها | حفظ تنظیمات و حساب‌ها هنگام نصب مجدد و حذف برنامه |
| عیب‌یابی محلی | تولید گزارش محدود بدون ارسال خودکار |
| فرایند ساخت | تولید آرشیوهای ⁦ARMv7⁩، ⁦ARM64⁩، ⁦x86-64⁩، ⁦MIPS⁩ و ⁦MIPS⁩ با ترتیب بایت معکوس |

مستندات و پیام‌های نصب به فارسی، انگلیسی آمریکایی، روسی و عربی در دسترس هستند. آرشیوهای نصب ⁦v0.2.0-rc.2⁩ هر چهار زبان را در نصب‌کننده دارند. رابط وب هستهٔ اصلی در این پروژه ترجمه نشده است.

## دریافت فایل

[نامزد انتشار v0.2.0-rc.2](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/tag/v0.2.0-rc.2) با ساخت موفق هر پنج معماری منتشر شده است.

| معماری | فایل نصب | کنترل سلامت |
| --- | --- | --- |
| ⁦armv7⁩ | [دریافت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz.sha256) |
| ⁦arm64⁩ | [دریافت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz.sha256) |
| ⁦amd64⁩ | [دریافت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz.sha256) |
| ⁦mips⁩ | [دریافت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz.sha256) |
| ⁦mipsle⁩ | [دریافت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz.sha256) |

**ساخت موفق به معنی تست روی روتر نیست.** آرشیوها نصب‌کنندهٔ برنامه هستند؛ فریم‌ور، بکاپ یا بستهٔ ⁦ipk/apk⁩ نیستند. معماری را از [راهنمای سازگاری](docs/COMPATIBILITY.fa.md) انتخاب کنید. نصب روی محیط‌های دیگر به گزینهٔ صریح `--experimental` نیاز دارد؛ پشتیبانی کلی از هیچ برندی ادعا نمی‌شود.

نسخه‌های در حال توسعه از [فرایند ساخت](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/build.yml) دریافت می‌شوند؛ موفقیت واقعی هر اجرا را بررسی کنید.

## محدودیت‌ها

- پراکسی ⁦TCP⁩ است؛ ⁦SOCKS5 UDP⁩ پشتیبانی نمی‌شود. برخی تماس‌ها و بازی‌ها کار نمی‌کنند.
- نسخهٔ هستهٔ استفاده‌شده، پورت‌های مقصد 80، 443، 8080 و 8443 را در مستندات ذکر کرده است.
- پنل با ⁦HTTP⁩ روی آدرس ⁦IPv4⁩ شبکه داخلی روتر در دسترس است؛ فقط از شبکهٔ مورد اعتماد استفاده کنید و آن را روی ⁦WAN⁩ باز نکنید. تنظیمات قبلی حفظ می‌شوند؛ آدرس‌های گوش‌دادن آن‌ها را بررسی کنید.
- ⁦PassWall2⁩ جداگانه تنظیم می‌شود. پیش از انتخاب نود، ⁦Localhost Proxy⁩ را خاموش کنید تا اتصال خود هسته وارد حلقه نشود.
- نسخهٔ قدیمی ⁦test1⁩ به مهاجرت جداگانه نیاز دارد؛ نصب کارای آن را با این نامزد انتشار جایگزین نکنید.

## مشارکت

[نتیجه تست موفق یا ناموفق](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose) را با مدل دقیق، نسخهٔ فریم‌ور، معماری، نسخهٔ بسته و نتیجهٔ هر آزمون ثبت کنید. گزارش فارسی، انگلیسی، روسی و عربی پذیرفته می‌شود.

[مشارکت](CONTRIBUTING.md) · [امنیت](SECURITY.md) · [تغییرات](CHANGELOG.md) · [ساخت از کد](docs/BUILDING.md)

## سازنده و مجوز

ساخت و نگهداری یکپارچه‌سازی ⁦OpenWrt:⁩ **شهرام معروف — ⁦Shahram Marouf⁩**.
توسعهٔ هسته: [armanlife/firefox-vpn-client](https://github.com/armanlife/firefox-vpn-client).
[مشخصات منبع اصلی](src/UPSTREAM.md) و [مجوز GPL-3.0](LICENSE) حفظ شده‌اند.

