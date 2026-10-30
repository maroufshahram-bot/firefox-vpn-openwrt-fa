# Build and package

Use the Go version declared by `src/core/go.mod` (currently 1.27.2), Python 3,
a POSIX shell, tar and sha256sum on a development machine. Dependency downloads
require network access. Do not edit the module version merely to force a build
with an older compiler. The pinned core is 1.0.5; see `src/UPSTREAM.md`.

```sh
sh packaging/build.sh armv7
python3 scripts/package.py armv7
```

Other targets: `arm64`, `amd64`, `mips`, `mipsle`. Binaries are Linux, CGO-disabled;
MIPS builds use software floating point. The build script bundles the corresponding
source and license. The packager normalizes archive order, ownership and timestamps,
and writes an archive checksum beside it in `dist/`. This makes packaging repeatable
for identical inputs; it does not claim independently verified reproducible Go builds.

The build workflow compiles each target and uploads separate artifacts. Main-branch candidate builds publish a prerelease only after every validation
and matrix build succeeds. VERSION must identify a release candidate; existing
release assets are never silently replaced. Optional `v*` tags must match VERSION. All published candidates remain unverified on devices
until reports support a stronger status. No firmware images or native ipk/apk
packages are generated in this iteration.

Host-side checks:

```sh
python3 -m unittest discover -s tests -v
python3 scripts/validate.py
```

These cover platform gating, archive metadata/ELF target checks, checksum rejection,
packaging repeatability, docs links and installer behavior in isolated Linux fixtures.
They do not prove OpenWrt service, Mozilla login, PassWall2 routing or reboot behavior.

فارسی: ساخت روی کامپیوتر با نسخه Go مشخص‌شده انجام می‌شود. خروجی‌ها نصب‌کننده برنامه هستند، نه فریم‌ور یا بسته مدیر نرم‌افزار. موفقیت آزمون‌های کامپیوتری جایگزین تست روتر نیست.
