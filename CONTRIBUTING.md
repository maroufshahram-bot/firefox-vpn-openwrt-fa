# Contributing / مشارکت

Persian and English contributions are welcome. Start with [device testing](docs/TESTING.md)
or its [Persian guide](docs/TESTING.fa.md). Report failed tests too: they define the
actual compatibility boundary.

- Search existing issues before opening a report. Use the bug or device-test form.
- Include exact firmware/model/package versions and pass/fail/not-tested outcomes.
- Do not attach account data, full logs, real network addresses or credentials.
- Keep patches focused. Describe the user-visible change and how it was verified.
- Run `python3 -m unittest discover -s tests -v` and `python3 scripts/validate.py`.
- Keep English and Persian instructions consistent. US English is the English style.
- Keep upstream core changes separate from OpenWrt integration changes and preserve GPL attribution.
- Never promote a device to tested status solely from a build or preflight result.

Maintainers review compatibility reports before updating the matrix. There is no
promised response time. Questions and feature ideas can use a GitHub issue; no
external chat account is required.

گزارش فارسی پذیرفته می‌شود. مشخصات دقیق محیط و نتیجهٔ تک‌تک تست‌ها را بنویسید. اطلاعات حساب، آدرس واقعی، رمز و لاگ کامل را منتشر نکنید. تغییر وضعیت سازگاری پس از بازبینی شواهد انجام می‌شود.
