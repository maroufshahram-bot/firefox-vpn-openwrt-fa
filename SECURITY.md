# Security / امنیت

This is an independent integration with an upstream core and an external account
service. It is not a security certification. Candidate releases require device testing.

Keep management access on a trusted LAN or an existing secure path. New installs
bind the dashboard to LAN IPv4 and the proxy to loopback; existing settings are
preserved and must be reviewed by their owner. Do not open the dashboard to WAN.

Do not post credentials, tokens, verification codes, account backups, real network
addresses, MACs, serial numbers or full logs in an issue or pull request.
The local diagnostics script deliberately reads only a small platform allowlist
and does not upload anything. Review its output before sharing.

If you find a vulnerability, use GitHub's **Report a vulnerability** option in the
repository Security tab if it is enabled. If private reporting is unavailable,
open an issue requesting a private reporting channel **without exploit details or
sensitive data**. No private reporting channel or response SLA is promised here.

فارسی: پنل را روی اینترنت باز نکنید و اطلاعات خصوصی را در Issue قرار ندهید. اگر گزارش خصوصی GitHub فعال است از آن استفاده کنید؛ در غیر این صورت فقط درخواست مسیر خصوصی بدهید و جزئیات آسیب‌پذیری یا اطلاعات حساس را عمومی نکنید.
