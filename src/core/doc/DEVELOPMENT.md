# Development Workflow

Choose a reading path from the [documentation index](README.md). The deeper references are [architecture](ARCHITECTURE.md), [authentication](AUTHENTICATION.md), [sessions](SESSIONS.md), and [configuration/state](CONFIGURATION.md).

## Establish the workspace

Run commands from the directory containing [go.mod](../go.mod). Historical audit paths describe one machine; do not assume they identify the current checkout.

The module is `github.com/armanlife/firefox-vpn-client/core`. The executable is `./cmd/firefox-vpn-core`; the control-plane clients live in `internal/` (`fxa`, `guardian`, `serverlist`, `controlplane`). See [ARCHITECTURE.md](ARCHITECTURE.md) for the layout. This source was an extracted snapshot without Git metadata during the audit. Check the current directory before using Git commands or proposing a branch workflow.

Runtime code lives in `cmd/firefox-vpn-core`, one file per responsibility, with each file's tests next to it. Use symbol searches and nearby tests before considering structural changes:

```powershell
rg -n 'RunRenewalLoop|adoptPass|updateCurrentSessionToken' internal/proxy
rg -n '^func TestProxyController' cmd/firefox-vpn-core
```

There is no generated-code step and no frontend build: the dashboard's plain HTML, CSS, and JavaScript in `internal/service/web/` are embedded with `go:embed`, so any dashboard change requires rebuilding the executable. See [DASHBOARD.md](DASHBOARD.md).

## Toolchain and dependencies

`go.mod` declares Go `1.27.2` (gVisor's minimum; with `GOTOOLCHAIN=auto` an older Go downloads it). It pins `quic-go v0.59.0`, `x/crypto`, `x/net`, `x/sys` (Windows API, registry), `x/term`, `fyne.io/systray v1.12.2` (tray icon; pulls `godbus/dbus` on Linux), `gvisor.dev/gvisor` (the userspace TCP/IP stack of the Android VPN mode, [internal/tun](../internal/tun)), `github.com/sagernet/sing-tun` with `sagernet/sing` and sagernet's gVisor fork (the desktop TUN, imported only by [internal/tunstack](../internal/tunstack); its stack needs the build tag `with_gvisor`), `github.com/sagernet/netlink` (on Linux, removing the rules a crashed TUN left), `github.com/Microsoft/go-winio` (the TUN service's named pipe), and `golang.org/x/mobile` at commit `8b95e45f8d3e` (2026-09-08, kept by a `tool` directive) for the Android build of [mobile/](../mobile). None of them needs CGO on Windows or Linux; the Android build uses CGO through gomobile and the NDK.

Android: `gomobile` and `gobind` are installed at the same x/mobile commit (`go install golang.org/x/mobile/cmd/gomobile@8b95e45f8d3e`, likewise `cmd/gobind`). With `ANDROID_HOME`, `ANDROID_NDK_HOME`, and a JDK (Android Studio's) set, `gomobile bind -target=android -androidapi 24 -trimpath -ldflags "-s -w" -o ../app/android/app/libs/core.aar ./mobile` builds the library the Flutter app links. Read current [go.mod](../go.mod) and [go.sum](../go.sum) before changing dependencies.

Control-plane requests and the proxy's CONNECT carry desktop Firefox's User-Agent for this system ([controlplane/headers.go](../internal/controlplane/headers.go)), with the Firefox release from Mozilla's product details; it is not this program's version.

```powershell
go version
go mod download
go mod verify
```

Do not run `go mod tidy` or upgrade dependencies for an unrelated edit. Dependency changes need an explicit compatibility review of H2/H3 behavior and the supported Go version.

## Edit and verification loop

1. Define the requested behavior and inspect its source, callers, and tests.
2. Use [CHANGE-GUIDE.md](CHANGE-GUIDE.md) to identify configuration, lifecycle, and documentation surfaces.
3. Make the smallest coherent change; preserve unrelated working configuration.
4. Format touched Go files with `gofmt -w`.
5. Run focused behavior tests. For Go changes, finish with the normal suite and vet when appropriate.
6. Build the executable when the change affects compilation or distribution.
7. Update the owning document and report passed, failed, skipped, and missing verification.

Normal checks:

```powershell
go test -count=1 -timeout=120s ./...
go vet ./...
```

See [TESTING.md](TESTING.md) for focused commands, isolation, race requirements, and live opt-ins. Documentation-only changes need link/language/source checks; they do not require a live account or proxy process.

## Windows development build

Use a separate output name while developing so the supplied/running executable is not replaced. The executable's icon and version information come from `cmd/firefox-vpn-core/rsrc_windows_*.syso`, which `go build` links in. They are build output, not in Git: `scripts/build-windows.ps1` makes them with the release version (`APP_VERSION=<version> go generate ./cmd/firefox-vpn-core`, [tools/winres](../tools/winres/main.go)); a development build without them has no icon:

```powershell
New-Item -ItemType Directory -Path dist -Force | Out-Null
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -o dist\firefox-vpn-core-dev.exe ./cmd/firefox-vpn-core
go version -m dist\firefox-vpn-core-dev.exe
Get-FileHash -LiteralPath dist\firefox-vpn-core-dev.exe -Algorithm SHA256
```

These environment values persist in the current PowerShell process. Use a fresh shell or restore them before a different target or a race check. The application itself needs neither Go nor curl at runtime.

A development build next to the distribution executable shares its `user-data` folder (the folder next to the executable). Run test builds from a separate folder or with `-data-dir` and a temporary home so they never touch real accounts. Replacing `dist\firefox-vpn-core.exe` or restarting a running instance is a separate concrete action within the user's requested scope. Confirm listener ownership, and stop test instances by PID or path rather than image name.

The Windows app carries `d3dcompiler_47.dll`, DirectX's shader compiler, beside `firefox-vpn.exe`: Flutter's renderer (ANGLE on Direct3D 11) needs it to draw anything, and some stripped copies of Windows 10 (the "lite" ones) lack the system's copy, where the window stays empty (black in dark mode). `app\windows\CMakeLists.txt` installs it from the Windows SDK (`Windows Kits\10\Redist\D3D\x64`, which Visual Studio's C++ workload installs), and `scripts\build-windows.ps1` refuses a build without it. Checked on 2026-10-25: a copy of the app with a broken `d3dcompiler_47.dll` beside it opened an empty window; with the SDK's copy beside it, it drew normally.

The TUN service is built with the gVisor tag and without a console: `go build -tags with_gvisor -ldflags "-H windowsgui ..." ./cmd/firefox-vpn-tun` (`scripts/build-windows-tun.ps1`; `APP_VERSION=<version> go generate ./cmd/firefox-vpn-tun` makes its resources, `tools/winres -program tun`). The app's build puts it beside the core in `core\`, from where the app or Setup installs it. On Linux and macOS it is `CGO_ENABLED=0 go build -tags with_gvisor ./cmd/firefox-vpn-tun`; `scripts/build-linux.sh` puts it in the app's `core/`, `scripts/build-macos.sh` in `Contents/MacOS` (both processors in one file, signed ad hoc like the core).

The TUN mode's code for Linux and macOS (`*_linux.go`, `*_darwin.go`, `*_unix.go` in `internal/tunservice`, `internal/tunapi`, `internal/tunstack`, `internal/egress`) builds on Windows with `GOOS=linux` or `GOOS=darwin` (`go vet` too). Linux's tests run in WSL: `go test -exec <program>` runs each test binary through a program that hands it to `wsl.exe` (with the path under `/mnt/`), and a real TUN needs root there (`wsl -u root`, which changes only the WSL machine's network). The end-to-end test runs a real TUN with a stand-in for the core, as root: `go test -c -tags 'e2e with_gvisor' -o e2e.test ./internal/tunservice`, then `sudo ./e2e.test -test.run TestEndToEnd -test.v`; CI runs it on its Linux and macOS computers. WSL sends DNS to Windows (its server is on loopback), so there apps' names do not go through the TUN, and the test checks DNS through systemd-resolved instead.

### Signing key

One Ed25519 key signs updates: a release's `SHA256SUMS.txt` (`SHA256SUMS.txt.sig`), which the desktop app (Windows, Linux, macOS) checks before it installs a new version of itself ([internal/update](../internal/update)), and the TUN service's executable (`firefox-vpn-tun.exe.sig`, `firefox-vpn-tun.sig` beside it; in the macOS app `Contents/Resources/firefox-vpn-tun.sig`, since only code may be in `Contents/MacOS`), which the installed TUN service checks before it replaces itself ([internal/tunservice](../internal/tunservice)). The builds put its public half into the core and the TUN service (`-ldflags -X github.com/armanlife/firefox-vpn-client/core/internal/signing.Key=<base64>`); `scripts/signing-key.ps1` derives it from the private key in `FIREFOX_VPN_SIGNING_KEY` (base64, the GitHub Actions secret of the same name, which CI's Windows, Linux, and macOS builds and its release job use) or in the file named by `FIREFOX_VPN_SIGNING_KEY_FILE` (a developer key for local builds); `scripts/build-linux.sh` and `scripts/build-macos.sh` read the same two. The macOS script signs the TUN service's executable after `codesign`, then seals the app again without `--deep`, which leaves the executable as it was signed. [tools/sign](../tools/sign/main.go) makes a key (`go run ./tools/sign -keygen FILE`, which prints the public key) and the signatures (`go run ./tools/sign FILE...`). Keep private keys out of the repository (`*.signing-key` is ignored), chat, and logs. A build without a key takes no updates: the app offers the download and the service is installed again with the administrator prompt; a build accepts only updates signed with its own key, so a developer-key build and a release take no updates from each other.

### Releasing

A release is the version in `app/pubspec.yaml` and its tag `v<version>`, which CI builds and publishes. Give the version to the systems whose users should update in `.github/updates.json` (`windows`, `linux`, `macos`, `android`): the apps offer what it names for their system ([OPERATOR.md](OPERATOR.md#app-updates)), so a system that nothing changed for keeps its older version, and its users are not asked to download the same app again. Edit it once the release is published, not in the release's commit: the apps then find the new version's files at once. Name only a version whose release has that system's files (a release built with `PLATFORMS` may lack some): until it has them, that system's apps offer nothing and look again every hour (the About page's check says the version is not published yet). The apps read the file from the default branch, so it can change after a release, but a version an app has already found stays on offer for up to 5 days, and one it has downloaded installs at its next start: take a version back by publishing a newer one. CI fails a file with other keys, other values, or a version newer than `app/pubspec.yaml`, and warns when a release's version is given to no system.

### Trying the app's updates locally

`scripts/build-windows.ps1 -AppVersion <older version>` builds an older-looking copy; a build of the current version is the update. A local release folder holds `v<version>/` with the versioned zip (and Setup), a `SHA256SUMS.txt` in `sha256sum` format, and its signature by the developer key (`go run ./tools/sign SHA256SUMS.txt`). Any small HTTP server on loopback that answers `/releases/latest` with GitHub's `{"tag_name": "v<version>", "html_url": ...}`, serves the folder under `/releases/download/`, and, to try the choice by system, serves an `updates.json` at `/updates.json`, stands in for GitHub: start the app with `FIREFOX_VPN_UPDATE_SERVER=http://127.0.0.1:<port>` (the app checks there; its core inherits the variable and downloads there, directly). Only one copy of the app runs at a time (the window's single-instance mutex), so close an installed copy before trying a portable one, and note that a test copy rewrites the autostart entry (`HKCU\...\Run`, value `Firefox VPN`) for itself. On Linux the release folder holds the versioned tar.gz of `scripts/build-linux.sh` instead (two builds with the developer key, the older one installed where `install.sh desktop` puts it). On macOS, `scripts/test-update-macos.sh` makes the whole round on CI's Mac, with a key made for the test; it replaces the app in `/Applications` and uses the app's own data folder, so it runs there only.

## Run modes

`runCommand` in [main.go](../cmd/firefox-vpn-core/main.go) picks one: the status screen or the plain log (no mode flag, `-plain`), the tray icon (`-tray`), the desktop app's core process (`-app`, started by the Flutter app in `app/`; contract in [OPERATOR.md](OPERATOR.md#desktop-app-mode)). The mode flags and one-shot commands are mutually exclusive. On Windows, `-tray` and `-app` apply `system_proxy.mode` to the signed-in user's real Internet Settings, and there is no mode that leaves them alone: `clear` (the default) undoes a setting of this program's at start and turns the system proxy off when the user switches to it, and `set` points Windows at the test instance while its proxy runs. Run them on the real desktop only when that change is acceptable for the task. To try `-app` by hand, start a development build with an isolated `-data-dir` and non-default ports, keep its standard input as an open pipe, and stop it by closing that pipe or with `POST /api/app/exit`. A failed `-app` start writes `app-error: <code> <detail>` as the first line on the standard error (`dashboard_port_in_use` exit 4, `already_running` exit 3, `start_failed` exit 1); the codes, details, and exit statuses are the app's contract in [OPERATOR.md](OPERATOR.md#app-error-line), written by `desktop.ReportAppError`.

## Operational testing

Use [OPERATOR.md](OPERATOR.md) for login and startup commands. Local fixtures do not require Mozilla credentials. Real-account testing can send traffic, activate account access, or persist refreshed credentials; follow the explicit live-test workflow in [TESTING.md](TESTING.md).

Observe the actual listener, upstream socket, and response together when attributing live traffic. A changed IP or another proxy's success alone is insufficient.

## Completing a task

Describe the behavior changed, affected files, checks and skips, state/configuration impact, and remaining limits. Do not turn an untested possibility into a confirmed feature.
