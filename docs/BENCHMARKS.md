# Design references

Reviewed 2026-10-30. These are design references, not affiliations or proof that
copying a repository pattern will cause adoption. No project code was copied.

| Reference | Observed practice | Application here |
| --- | --- | --- |
| [OpenClash README](https://github.com/vernesong/OpenClash/blob/master/README.md) and [releases](https://github.com/vernesong/OpenClash/releases) | Quick start, download choices, dependencies, build instructions, upstream credits | Short landing README, separate install/build guides, explicit application archive format and attribution |
| [LuCI App for Tailscale Community](https://github.com/Tokisaki-Galaxy/luci-app-tailscale-community/blob/master/README.md) | Language switch, firmware prerequisites, LuCI integration and build workflow | Parallel Persian/English entry points, service requirements, workflow artifacts |
| [AdGuard Home README](https://github.com/AdguardTeam/AdGuardHome/blob/master/README.md) | Separate stable/beta/edge instructions and contribution paths | Release candidate status distinct from device-tested runtime evidence |
| [AdGuard Home bug form](https://github.com/AdguardTeam/AdGuardHome/blob/master/.github/ISSUE_TEMPLATE/bug.yml) | Structured platform, version, installation, expected/actual result fields | Bilingual bug/test forms with per-step outcomes and a privacy check |

We deliberately keep experimental installation opt-in and exclude full account/log
uploads. Initial goals: independent successful installs, useful failure reports,
reviewed device combinations and external contributions. Stars are secondary.
Do not publish fabricated counters or mark a device tested from a successful build.

فارسی: الگوها برای ساده‌شدن نصب، روشن‌بودن وضعیت انتشار و دریافت گزارش قابل استفاده انتخاب شده‌اند. هیچ نتیجهٔ تضمینی درباره رشد پروژه یا سازگاری دستگاه‌ها از آن‌ها استنتاج نمی‌شود.
