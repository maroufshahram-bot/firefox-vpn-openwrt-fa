<div dir="rtl">

# ‏Firefox VPN لـ OpenWrt

**تكامل مستقل لعميل Firefox VPN غير الرسمي مع OpenWrt.**

[English](README.md) · [فارسی](README.fa.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [التثبيت](docs/INSTALL.ar.md) · [التوافق](docs/COMPATIBILITY.ar.md) · [إرسال نتيجة اختبار](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose)

[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Validation](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml/badge.svg)](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml)

يشغّل المشروع [النواة الأصلية](https://github.com/armanlife/firefox-vpn-client) كخدمة على OpenWrt، ويتيح فتح لوحتها من LuCI واستخدام وكيل SOCKS محلي مع مصادقة عبر PassWall2. لا يلزم تثبيت متصفح Firefox أو بيئة سطح مكتب على الموجّه. **يلزم حساب Mozilla / Firefox.**

المشروع غير تابع لموزيلا ولا يحظى بتأييد رسمي منها. أهلية الحساب وتوفّر الخدمة والسرعة والحصص واستمرار الوصول تحدّدها الخدمة الأصلية.

## ابدأ هنا

1. اقرأ [جدول التوافق](docs/COMPATIBILITY.ar.md) قبل التنزيل.
2. اتبع [دليل التثبيت](docs/INSTALL.ar.md)، بما فيه منع حلقة الاتصال في PassWall2.
3. نفّذ [قائمة اختبارات الجهاز](docs/TESTING.ar.md) وشارك النتيجة.

**الحالة الحالية: إصدار مرشّح.** أبلغ مالك جهاز Google WiFi (Gale) واحد عن عمل المكوّنات، بما في ذلك بعد إعادة التشغيل. ما زال المثبّت المتكامل الجديد وجميع تركيبات الأجهزة والبرامج الثابتة الأخرى بحاجة إلى اختبار فعلي. نجاح البناء لا يثبت توافق الجهاز.

## المكوّنات

| المكوّن | الغرض |
| --- | --- |
| الفحص المسبق | فحص عائلة البرنامج الثابت والمعالج والملف التنفيذي وLuCI والمساحة المتاحة |
| خدمة OpenWrt | التشغيل عند الإقلاع وإعادة تشغيل النواة بعد توقف غير متوقع عبر procd |
| قائمة LuCI | فتح اللوحة من Services → Firefox VPN |
| حفظ الحسابات | الاحتفاظ بالإعدادات والحسابات عند إعادة التثبيت والإزالة |
| تشخيص محلي | تقرير محدود بحقول مسموح بها، دون رفعه |
| بناء للمجتمع | أرشيفات ARMv7 وARM64 وx86-64 وMIPS وMIPS little-endian |

تتوفر الأدلة ورسائل المثبّت بالإنجليزية الأمريكية والفارسية والروسية والعربية. لا يترجم هذا المشروع لوحة المشروع الأصلي. تتضمن أرشيفات v0.2.0-rc.2 لغات المثبّت الأربع.

## التنزيل والبناء التجريبي

يتوفر [الإصدار المرشّح v0.2.0-rc.2 لاختبار المجتمع](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/tag/v0.2.0-rc.2). اجتازت أرشيفات المعالجات الخمسة البناء المتقاطع وفحوص التغليف.

| المعالج | الأرشيف | التحقق من السلامة |
| --- | --- | --- |
| armv7 | [تنزيل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-armv7.tar.gz.sha256) |
| arm64 | [تنزيل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-arm64.tar.gz.sha256) |
| amd64 | [تنزيل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-amd64.tar.gz.sha256) |
| mips | [تنزيل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mips.tar.gz.sha256) |
| mipsle | [تنزيل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.2/firefox-vpn-openwrt-v0.2.0-rc.2-linux-mipsle.tar.gz.sha256) |

**نجاح البناء ليس اختباراً للجهاز.** الأرشيفات مثبّتات تطبيق، وليست صور برامج ثابتة أو نسخاً احتياطية أو حزم `.ipk` أو `.apk`. اختر المعالج وفق [دليل التوافق](docs/COMPATIBILITY.ar.md). تتطلب المنصات الأخرى قبولاً صريحاً بالخيار `--experimental`؛ لا يعني ذلك دعم علامة تجارية بالكامل.

تتوفر ملفات التطوير من [عمليات البناء الناجحة](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/build.yml). تحقّق من نتيجة العملية قبل التنزيل.

## القيود

- الوكيل يدعم TCP فقط؛ لا يدعم SOCKS5 UDP. قد لا تعمل بعض المكالمات والألعاب.
- توثّق نسخة النواة المثبّتة منافذ الوجهة 80 و443 و8080 و8443.
- تستخدم اللوحة HTTP على عنوان LAN IPv4 للموجّه. استخدم شبكة موثوقة ولا تكشف اللوحة على WAN. تُحفظ الإعدادات الحالية؛ راجع عناوين الاستماع.
- يُضبط PassWall2 بصورة منفصلة. عطّل Localhost Proxy قبل اختيار العقدة لمنع النواة من إعادة توجيه اتصالها إلى نفسها.
- تحتاج تثبيتات `test1` القديمة إلى ترحيل منفصل؛ لا تستبدل تثبيتاً يعمل بهذا الإصدار المرشّح.

## ساعد في اختبار التوافق

[أرسل نتيجة الاختبار الناجح أو الفاشل](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose)، مع الطراز الدقيق ونسخة البرنامج الثابت والمعمارية ونسخة الحزمة ونتيجة كل خطوة. تُقبل التقارير باللغات الأربع.

راجع [المساهمة](CONTRIBUTING.md) و[الأمان](SECURITY.md) و[سجل التغييرات](CHANGELOG.md) و[تعليمات البناء](docs/BUILDING.md) بالإنجليزية.

## المساهمون والترخيص

أنشأ المشروع ويصونه **Shahram Marouf**. تطوّر النواةَ جهةُ [armanlife/firefox-vpn-client](https://github.com/armanlife/firefox-vpn-client)، ويوفر هذا المستودع تكامل OpenWrt والتغليف والتوثيق. راجع [مصدر الشيفرة](src/UPSTREAM.md). يُوزّع بموجب [GPL-3.0](LICENSE).

</div>
