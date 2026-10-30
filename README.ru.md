# Firefox VPN для OpenWrt

**Независимая интеграция неофициального клиента Firefox VPN с OpenWrt.**

[English](README.md) · [فارسی](README.fa.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [Установка](docs/INSTALL.ru.md) · [Совместимость](docs/COMPATIBILITY.ru.md) · [Сообщить о тесте](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose)

[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Validation](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml/badge.svg)](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/validate.yml)

Проект запускает [исходное ядро](https://github.com/armanlife/firefox-vpn-client) как службу OpenWrt, открывает его панель из LuCI и предоставляет локальный SOCKS-прокси с аутентификацией для PassWall2. Браузер Firefox и графическая среда на роутере не нужны. **Требуется учётная запись Mozilla / Firefox.**

Проект не связан с Mozilla и не одобрен ею. Условия доступа, доступность сервиса, скорость, квоты и дальнейшая работа зависят от исходного сервиса.

## С чего начать

1. Перед загрузкой прочитайте [таблицу совместимости](docs/COMPATIBILITY.ru.md).
2. Выполните [инструкцию по установке](docs/INSTALL.ru.md), включая защиту от зацикливания в PassWall2.
3. Пройдите [проверки на устройстве](docs/TESTING.ru.md) и отправьте результат.

**Текущий статус: кандидат в релиз.** Владелец одного Google WiFi (Gale) сообщил об успешной работе компонентов, в том числе после перезагрузки. Новый комплексный установщик и все остальные сочетания устройств и прошивок ещё требуют испытаний на реальном оборудовании. Успешная сборка не подтверждает совместимость устройства.

## Возможности

| Компонент | Назначение |
| --- | --- |
| Предварительная проверка | Семейство прошивки, процессор, исполняемый файл, LuCI и свободное место |
| Служба OpenWrt | Автозапуск и перезапуск ядра после неожиданного завершения через procd |
| Пункт меню LuCI | Доступ к панели через Services → Firefox VPN |
| Постоянное хранение аккаунтов | Сохранение настроек и аккаунтов при переустановке и удалении |
| Локальная диагностика | Краткий отчёт только из разрешённых полей без отправки данных |
| Сборки для сообщества | Архивы ARMv7, ARM64, x86-64, MIPS и MIPS little-endian |

Руководства и сообщения установщика доступны на английском (США), персидском, русском и арабском. Панель исходного проекта здесь не переводится. В опубликованных архивах v0.2.0-rc.1 установщик поддерживает только английский и персидский; русский и арабский добавлены в исходный код для следующей сборки.

## Загрузка и экспериментальные сборки

Доступен [кандидат v0.2.0-rc.1 для тестирования сообществом](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/tag/v0.2.0-rc.1). Все пять архивов прошли кросс-компиляцию и проверку упаковки.

| Процессор | Архив | Проверка целостности |
| --- | --- | --- |
| armv7 | [Скачать](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-armv7.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-armv7.tar.gz.sha256) |
| arm64 | [Скачать](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-arm64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-arm64.tar.gz.sha256) |
| amd64 | [Скачать](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-amd64.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-amd64.tar.gz.sha256) |
| mips | [Скачать](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-mips.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-mips.tar.gz.sha256) |
| mipsle | [Скачать](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-mipsle.tar.gz) | [SHA-256](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/releases/download/v0.2.0-rc.1/firefox-vpn-openwrt-v0.2.0-rc.1-linux-mipsle.tar.gz.sha256) |

**Сборка не заменяет испытание на устройстве.** Архивы — установщики приложения, а не образы прошивки, резервные копии или пакеты `.ipk` / `.apk`. Выберите архитектуру по [руководству](docs/COMPATIBILITY.ru.md). Для других платформ требуется явный параметр `--experimental`; поддержка целых брендов не заявляется.

Артефакты разработки доступны в успешных [запусках сборки](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/actions/workflows/build.yml). Проверяйте результат конкретного запуска.

## Ограничения

- Только TCP; SOCKS5 UDP не поддерживается. Некоторые звонки и игры работать не будут.
- В закреплённой версии ядра документированы порты назначения 80, 443, 8080 и 8443.
- Панель работает по HTTP на LAN IPv4 роутера. Используйте доверенную сеть и не открывайте панель в WAN. Существующие настройки сохраняются; проверьте адреса привязки.
- PassWall2 настраивается отдельно. До выбора этого узла отключите Localhost Proxy, чтобы ядро не направляло собственное соединение обратно в себя.
- Для старой установки `test1` нужен отдельный перенос. Не заменяйте работающую установку этим кандидатом в релиз.

## Помогите проверить совместимость

[Отправьте результат успешного или неудачного теста](https://github.com/maroufshahram-bot/firefox-vpn-openwrt-fa/issues/new/choose): точная модель, прошивка, архитектура, версия пакета и результат каждого шага. Принимаются отчёты на всех четырёх языках.

См. [участие в проекте](CONTRIBUTING.md), [безопасность](SECURITY.md), [историю изменений](CHANGELOG.md) и [инструкции по сборке](docs/BUILDING.md) (на английском).

## Авторство и лицензия

Автор и сопровождающий — **Shahram Marouf**. Ядро разрабатывает [armanlife/firefox-vpn-client](https://github.com/armanlife/firefox-vpn-client); этот репозиторий содержит интеграцию OpenWrt, упаковку и документацию. См. [происхождение исходного кода](src/UPSTREAM.md). Лицензия [GPL-3.0](LICENSE).
