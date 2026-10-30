# Operator Guide

How to run, configure, and troubleshoot the service. The [dashboard guide](DASHBOARD.md) describes the web interface, [MULTI-ACCOUNT.md](MULTI-ACCOUNT.md) the account rules, and [CONFIGURATION.md](CONFIGURATION.md) every setting and file.

## Capabilities

- SOCKS5 CONNECT and an HTTP proxy on the same port, for IPv4, IPv6, and domain targets; no UDP ASSOCIATE or BIND. The desktop app adds a TUN mode that carries every app's TCP ([TUN mode](#tun-mode)), and on Windows, Linux, and macOS updates itself ([App updates](#app-updates)).
- Optional routing: the sites and networks of a region (Iran by default; also China, Russia, Afghanistan, Turkey, Indonesia, Brazil, or none), or any listed destination, directly through your own internet, the rest through Mozilla.
- Windows: v2rayN's choice of Set system proxy (while the proxy runs) or Clear system proxy (the default), applied by the tray, the desktop app's core process, or the status screen when the proxy runs in its window. The proxy runs from the start, except in the desktop app, which starts it off; the app and `POST /api/power` turn it on and off.
- One or more Firefox Accounts; the next account takes over when the active one reaches the switch threshold (default 95% of its quota; the last usable account goes on to the end of its quota), keeps open connections, and returns to rotation after the quota reset.
- Upstream HTTP/2 over TLS (recommended) or experimental HTTP/3 over QUIC to Mozilla's Fastly proxies; one multiplexed session or a small pool.
- Location selection compatible with Firefox: recommended location by default, version-gated records honored; every location usable, including those the server list marks locked.
- Web dashboard for accounts, settings, live logs, and connection tests; all data in a portable `user-data` folder next to the executable.
- Tray icon (Windows and Linux desktops) with status and quick actions; notifications on Linux desktops.

## Windows

1. Put `firefox-vpn-core.exe` in a folder you can write to (not `Program Files`), for example `C:\Tools\FirefoxVPN\`.
2. Double-click it. A window shows the status screen (below) and runs the proxy. The `user-data` folder appears next to the executable, and the log is written to `user-data\logs`, one file per day.
3. Choose **1 Open the dashboard** (or browse to <http://127.0.0.1:10820/app/>), add an account on the Accounts page, and use `127.0.0.1:10810` as the SOCKS5 proxy in your applications (in a browser: SOCKS v5 with remote DNS).

Closing the window (with ✕ or **0 Quit**) stops the proxy. To keep it running without a window, start it with `-tray` (a tray icon). When a copy already serves the data folder (the desktop app's core or a tray), double-clicking the program opens the status screen for that copy instead: closing the window then leaves it running, and **Exit Firefox VPN completely** stops it. It never starts a second copy. Beside the desktop app (`firefox-vpn.exe` in the same folder, or the bundled core in its `core` folder) the program uses the app's `user-data` folder beside `firefox-vpn.exe`, so double-clicking it shows the app's core.

### Status screen

Whenever the program runs in an interactive terminal (double-click, PowerShell, cmd, or a Linux terminal) without a command, it shows a status screen instead of the raw log: the state and location, the SOCKS5 and dashboard addresses, the active account with a usage bar, followed by a numbered list of actions. Type a number and press Enter:

```text
  What would you like to do?

     1  Open the dashboard in the browser
    ────────────────────────────────────────────────────────────
     2  Choose a server
     3  Test the connection
     4  Restart the proxy
     5  Turn on system proxy
    ────────────────────────────────────────────────────────────
     6  Set panel login
    ────────────────────────────────────────────────────────────
     7  Show the log
    ────────────────────────────────────────────────────────────
     0  Quit (stops the proxy)

  Enter a number and press Enter: _
```

The list adapts to the situation, so the numbers can shift: **Start Firefox VPN** replaces the proxy actions when nothing runs; **Turn on system proxy** (or **Turn off system proxy**) appears where the system proxy is supported (Windows, macOS, and GNOME on Linux), and the window applies the choice itself while the proxy runs in it; **Exit Firefox VPN completely** appears when the window controls a copy running elsewhere (a tray or the desktop app's core), not a copy in another terminal or under systemd. **0** closes the window: it stops the proxy that runs in the window, and leaves a copy running elsewhere running.

**Set panel login** is the way back in after a forgotten panel password: **1** gives the panel a random username and password and shows them once (write them down); **2** asks for your own username and password (the password hidden as you type, at least six characters). Every browser signed in is signed out.

**Choose a location** lists 1 Always the fastest, 2 Recommended, 3 Test the real delay again, and then every measured location from 4 on, lowest latency first; 0 goes back. In the log, 0 goes back.

Backspace corrects the number, Esc clears it (or goes back), and Ctrl+C closes the window. Persian and Arabic digits work as well. While a task runs (a measurement can take a minute), other choices wait with a message; exiting never waits.

When nothing else serves the data folder, the proxy runs in the window and **0** stops it, on every system. `-plain` prints the classic log instead, and a terminal that is not interactive (a pipe, systemd, or a redirect) always gets the plain log. The screen needs ANSI colors (Windows 10 1511 or later).

### Version

The version (`major.minor.patch`, for example `1.1.0`) is shown on the status screen, at the bottom of the dashboard's sidebar, and by `firefox-vpn-core -version`. When a newer version serves the data folder (a copy in a terminal, after an update), an older tray icon keeps controlling it and says "served by version …" in its tooltip, but never starts the proxy itself again, so older code does not take over a folder a newer version wrote. When that copy stops, the older icon closes, so the new file's tray or the desktop app can take the folder; start the tray of the new file.

### Tray icon

The icon is Mozilla VPN's logo; a dot in its lower right corner tells the state:

| Icon | Meaning |
| --- | --- |
| Green dot | Proxy running (faded while asleep) |
| Blue dot | Proxy running, Windows' system proxy points at it (Set system proxy) |
| The connecting frame (no dot) | Starting, waiting (no account yet, or every account waits for its quota reset), or turned off (tooltip "Proxy off") |
| Red dot | Error; the tooltip gives a short reason, the dashboard and the status screen the whole message |
| No dot | The service is not reachable (stopped, or still starting) |

The menu follows v2rayN's: **Open dashboard**, the system proxy modes (**Clear system proxy**, **Set system proxy**; the current one is checked), **Restart proxy**, and **Exit Firefox VPN (stops the proxy)**, which reads **Close icon (the service keeps running)** when another copy (one in a terminal) runs the proxy. It is the native Windows menu: a menu drawn like v2rayN's (a WPF program) would need a UI framework and make the program much larger. v2rayN's subscription items do not apply here. Hovering over the icon shows the connection state and, when connected, where the traffic leaves: the city and country of the public IP (for example "Firefox VPN: Connected · Amsterdam, Netherlands"), which also names Mozilla's recommended location; while waiting or after an error it gives a short reason ("Waiting: No usable account", "Error: The proxy's port is in use"), and "Proxy off" when the proxy was turned off; details are on the dashboard and the status screen. The dashboard's Help page explains the icon colors and the system proxy modes. A left click opens the dashboard. On Windows the tray shows no notifications: the icon and its tooltip carry the state, the tooltip ends in "sign-in needed" while an account must sign in again, and it shows why the tray's own proxy could not start or stopped ("Could not start: …", until the service answers) and the error of a menu action that failed (for two minutes). Every such error is also written to the log. (A Windows notification would need a hidden script host such as PowerShell, which antivirus heuristics flag.) On a Linux desktop, notifications report account switches, errors, waiting states, and accounts that must sign in again.

When another copy already serves the data folder (one in a terminal), the tray only displays and controls it, and closing the icon leaves the proxy running. Otherwise the tray runs the proxy itself, and **Exit Firefox VPN** stops it. Only one tray (or desktop app core) runs per data folder; starting another tray exits quietly.

The program does not add itself to sign-in. To show the tray at sign-in, put a shortcut to `firefox-vpn-core.exe -tray` in the Startup folder (`shell:startup`); a console window flashes briefly before the icon appears. The desktop app has its own start-at-sign-in setting.

## Linux and systemd

On a desktop, run the binary from a writable folder like on Windows. On a server, install it as a service:

```bash
sudo ./scripts/install-systemd.sh install            # builds with Go, or BINARY=/path/to/firefox-vpn-core
sudo ./scripts/install-systemd.sh add-account        # terminal sign-in (email code), then restarts
sudo ./scripts/install-systemd.sh status
sudo ./scripts/install-systemd.sh logs
```

On a Linux desktop, `./firefox-vpn-core -tray` shows the tray icon (the desktop needs a StatusNotifierItem/AppIndicator host; GNOME needs the AppIndicator extension). To show it at sign-in, add `firefox-vpn-core -tray` to the desktop's autostart programs. Notifications use `notify-send` when it is installed.

The program and its data live in `/opt/firefox-vpn/` (`firefox-vpn-core` owned by root, `user-data/` owned by the `firefox-vpn` user). The unit runs with `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `NoNewPrivileges`, and write access only to the data folder, and restarts on failure.

The dashboard page is at `/app/` (`http://<address>:10820/app/`); the site root shows a plain "Up and running" page that does not name the program. A new server installed with the repository's `install.sh` opens the dashboard and the proxy to the network: it asks for the panel's username (left empty, a random one such as `panel5354`), then `setup-server` (`firefox-vpn-core -setup-server <user>`) sets `dashboard.listen` and `proxy.listen` to `0.0.0.0` on their ports, and gives each its own login with a random 16-character password: the dashboard's for that username, and a SOCKS5/HTTP proxy login with a random username such as `proxy4827` ("proxy" and four digits), so the proxy can be shared on the local network, and `install.sh` prints both logins and then the local and public panel addresses. It does not touch any firewall: it reminds to allow the two TCP ports in the server's firewall and the cloud provider's. An update keeps the addresses and logins.

`sudo firefox-vpn update` (also the last item of the `sudo firefox-vpn` menu) updates such a server: it asks GitHub which release is the newest (where `releases/latest` leads) and, when that one is newer than the installed core's `-version`, runs the `install.sh` from the repository's main branch with that release, as a rerun of the install command would: it downloads the release's core, checks it against the release's `SHA256SUMS.txt`, replaces the program and the `firefox-vpn` command, restarts the service, and keeps the data folder (accounts, settings, logins). When the installed core is as new, it only says so. `INSTALLER_URL` and `RELEASES_URL` replace the two addresses (tests, mirrors).

The dashboard opens to other devices only with a login: a non-loopback `dashboard.listen` without `dashboard.username` makes it listen on `127.0.0.1` with a warning, and settings that would open it without a login are refused. Without a login, reach it through an SSH tunnel (`ssh -L 10820:127.0.0.1:10820 server`). To set or change the login:

```bash
sudo ./scripts/install-systemd.sh panel
```

The dashboard has no HTTPS: its traffic, including passwords typed into it, crosses the network unencrypted.

`uninstall` (menu: **Uninstall**) asks first on a terminal, and whether to delete `/opt/firefox-vpn` with the accounts and settings; without a terminal it keeps them unless `PURGE=1`. It removes the `firefox-vpn` command too.

### OpenWrt

OpenWrt has no systemd, and `install.sh` needs bash: [docs/OPENWRT.md](../../docs/OPENWRT.md) is the guide for it, in English, for the people who run it. The core runs there from `/root/firefox-vpn/` under procd, OpenWrt's service manager, with an init script (`/etc/init.d/firefox-vpn`, `-plain`, `respawn`); devices use it as a proxy after `-setup-server`. Only the releases' amd64 and arm64 builds are supported (`x86_64` and `aarch64` routers): the 32-bit MIPS and ARM routers rarely have the room for the 14 MB program or the speed for its encryption, so the guide leaves them out (the core still compiles for them). Checked on 2026-10-25: the init script under OpenWrt 25.12.6's procd (`enable`, `start`, the log in `logread`, the proxy on from the start, a restart after `kill -9`, `stop`, `disable`), the guide's download and setup steps with OpenWrt's `wget` and `tar`, and the release's arm64 build installed by the guide on a real router (MediaTek MT7622, OpenWrt 24.10.6), where LuCI's Startup page lists it enabled and running and the panel opens from the network ([TESTING.md](TESTING.md)).

## Command line

| Command | Effect |
| --- | --- |
| `firefox-vpn-core` | Run the service (dashboard + proxy) with the status screen in an interactive terminal, or the plain log otherwise |
| `-plain` | Print the log instead of the status screen |
| `-version` | Print the version and exit |
| `-tray` | Show the tray icon; it runs the proxy itself unless another copy already serves the data folder |
| `-app` | Run the service for the desktop app, which starts the core this way; see [Desktop app mode](#desktop-app-mode) |
| `-update` | Install the desktop app's downloaded update: run only by the app's core, from a copy of itself (see [App updates](#app-updates)) |
| `-add-account` | Sign in on the terminal and store the account; restart a running service afterwards |
| `-list-accounts` | Print accounts, status, and usage |
| `-status` | Print the running service's proxy state, active account (masked), and data left in a few lines (`firefox-vpn status` on a server, under the service's own and autostart lines) |
| `-panel` | Set the panel username and password (a random one for each left empty, shown once); restart afterwards |
| `-setup-server USERNAME` | For a new headless server (`install.sh` runs it): open the dashboard and the proxy to the network: the dashboard login for USERNAME (`-` for a random one, `panel` and four digits) and a random proxy login (`proxy` and four digits), each with a random password; print the logins as `key=value` lines; restart afterwards |
| `-data-dir PATH` | Use another data folder |
| `-verbose` | Log CONNECT destinations and debug lines regardless of the config |

Settings live in `config.json` and the dashboard, not in flags.

## Desktop app mode

The desktop app starts the core as a child process with `-app [-data-dir DIR] [-verbose]` when it opens and talks to it only through the dashboard API with the `X-Local-Token` header (the token in `user-data/local-api.token`). In this mode the process:

- runs the service (dashboard and proxy) in-process, with no tray icon and no status screen; on Windows it closes a console window that was opened for it;
- starts with the proxy off (`Daemon.Off`), so nothing connects and the system proxy is not set until the user presses the button in the app (`POST /api/power`); the tray and the terminal (also under systemd) start on. Typed in a terminal, `-app` says so on the standard error: the mode for a terminal or a service is the core without `-app`;
- logs only to the day's file in `user-data/logs`, because its standard output and error are pipes the app may never read; an error that stops it is also written to the standard error (see [App error line](#app-error-line)) before it exits;
- is the only desktop host of the data folder: it shares the single-instance lock with the tray icon, and refuses to start when a tray, another `-app` copy, or a copy whose dashboard API answers (in a terminal or under systemd) already serves the folder;
- refuses to start when `dashboard.enabled` is false, since the app needs the API;
- applies `system_proxy.mode` like the tray (it reads the status every two seconds) and restores the previous Windows proxy setting when it stops; during auto-connect (below) it waits for a passed connection test first;
- reports `can_exit: true` in `GET /api/status`.

The dashboard API answers as soon as the process has loaded `config.json` and the account list (tens of milliseconds; nothing waits for the network). The proxy then starts asleep: it listens on `proxy.listen` at once and `GET /api/status` reports `state.phase` `running` with `proxy.sleeping_since` set, but nothing is sent to Mozilla until a client connection through Mozilla, `POST /api/test`, or a real-delay test needs it (see [Sleeping when unused](#sleeping-when-unused)). With `system_proxy.mode` `set`, the Windows proxy is applied right away, since the listener is open.

It stops cleanly (system proxy restored, proxy and dashboard stopped) on `POST /api/app/exit` with the local token, on Ctrl+C or SIGTERM, when its standard input ends, and on Windows when the user signs out or the computer shuts down or restarts. For the last, a hidden window receives `WM_ENDSESSION`, and the core restores the system proxy before answering it (Windows ends the process soon after the answer; after 3 seconds the proxy setting is restored directly), so applications are not left pointed at a closed port at the next sign-in. The app keeps the write end of the child's standard input pipe open for its whole life, so when the app quits or crashes the system closes it and the core follows. Anything written to the standard input is ignored. Only a pipe (or socket) is watched: with a terminal, the null device, or a file as standard input, the process runs until the exit request or a signal, and logs a warning.

| Exit status | Meaning |
| --- | --- |
| 0 | Stopped on request (exit API, end of standard input, signal, end of the Windows session) |
| 1 | Error: the service could not start (for example the dashboard is disabled, or `config.json` is invalid) |
| 2 | Bad arguments from the flag parser (an unknown flag or a stray argument); the usage text is written to the standard error, without an app error line |
| 3 | Another copy already serves the data folder; the app can use it through the API instead |
| 4 | Another program holds `dashboard.listen`; the app can offer another port and write it to `config.json` before starting again |

### App error line

When an `-app` run fails, the first line it writes to the standard error is machine-readable:

```text
app-error: <code> <detail>
```

The app reads lines that start with `app-error: `; `<code>` is one word, and `<detail>` is the rest of the line (it may contain spaces; line breaks in it are replaced with spaces).

| Code | Exit status | Detail | Lines |
| --- | --- | --- | --- |
| `dashboard_port_in_use` | 4 | The `dashboard.listen` address that is taken | This line only |
| `already_running` | 3 | The data folder | Followed by one line for people (`firefox-vpn-core: another copy …`) |
| `start_failed` | 1 | The error message | This line only; also for a mode conflict such as `-app -plain` |

A dashboard address held by another copy of this program that serves the same data folder is `already_running` (its API answers with this folder's token); one held by anything else, including a copy for another data folder, is `dashboard_port_in_use`. A taken proxy address does not stop the process: the dashboard runs and `GET /api/status` reports `state.code` `proxy_port_in_use` ([DASHBOARD.md](DASHBOARD.md#api)).

## TUN mode

The desktop app's third connection mode, **TUN** (`system_proxy.mode` `tun`), sends every app's traffic through the VPN, not only programs that use a proxy: store apps, WSL2 on Windows (NAT and mirrored networking), containers and virtual machines on Linux, terminal tools such as `curl`, `git`, and `pip`, and browsers. Only the desktop app has it, on Windows, Linux, and macOS; the core in a terminal, the tray copy, and the service on a server do not. Mozilla carries TCP only: other UDP is refused at once (QUIC falls back to TCP; calls and games that need UDP do not work, unless their program [bypasses the VPN](#programs-that-bypass-the-vpn)), and Mozilla's port rule still applies ([below](#a-site-on-an-unusual-port-does-not-open); `github.com:22` is rewritten to `ssh.github.com:443`).

The TUN runs in a separate program, the TUN service `firefox-vpn-tun` (`firefox-vpn-tun.exe` on Windows), which runs with the system's rights; the core stays an ordinary program.

| | Windows | Linux | macOS |
|---|---|---|---|
| The service | `FirefoxVPNTun` ("Firefox VPN TUN") | the systemd unit `firefox-vpn-tun.service` | the launchd daemon `com.arman.firefoxvpn.tun` |
| Its folder (with its log in `logs`) | `C:\Program Files\Firefox VPN\` | `/opt/firefox-vpn-tun/` | `/Library/Application Support/Firefox VPN TUN/` |
| The adapter | "Firefox VPN" (wintun) | `firefox-vpn` | the first free `utunN` |
| The core's channel to it | the named pipe `\.\pipe\firefox-vpn-tun` | the socket `/run/firefox-vpn-tun.sock` | the socket `/var/run/firefox-vpn-tun.sock` |
| Installing it asks with | Windows' administrator prompt | polkit's (`pkexec`) | the administrator's password |

While the mode is on and the proxy runs, the service:

- creates the adapter (`198.18.0.1/30` and `fdfe:dcba:9876::1/126`, MTU 9000) and routes everything into it. On Windows these are `0.0.0.0/0` and `::/0`. On Linux they are a routing table of their own (2026) with policy rules (priorities 9100 to 9110), which keep the local networks' routes, sockets bound to an interface, and the replies to connections from outside on the network (an SSH session to the computer keeps working). On macOS they are ranges more specific than the default route, which the local networks' routes still are. When the adapter gets no IPv6 address (IPv6 turned off), IPv6 is blocked instead, never leaked: by filters on Windows, an unreachable rule on Linux, blackhole routes for `::/1` and `8000::/1` on macOS;
- answers every DNS query (UDP or TCP port 53, to any address) itself with fake addresses from `198.18.0.0/15` that stand for names (`internal/tun` FakeDNS), and keeps DNS from leaving outside the TUN, so no query reaches the network's (possibly filtered) DNS servers. On Windows, Windows Filtering Platform filters block it. On Linux the rules send every packet to port 53 into the TUN, and systemd-resolved, where it runs, is given the TUN's DNS server for every name (`~.`). On macOS every network service's DNS servers are the TUN's (`198.18.0.2`) while it runs, as `networksetup -setdnsservers` sets them, and are set back afterwards from a backup in the service's folder (after a crash, when the service starts again);
- hands every TCP connection, with the name the fake address stands for, to a private SOCKS5 listener the core opens for the session (a random loopback port and login), so routing, accounts, and Mozilla apply as for any proxy client; only the traffic of the programs that bypass the VPN it sends directly itself ([below](#programs-that-bypass-the-vpn)).

The core's own connections (Mozilla, the account and server-list services, DNS over HTTPS, destinations routed directly) are bound to the network interface that has the default route besides the TUN (`internal/egress`: `IP_UNICAST_IF` on Windows, `SO_BINDTOIFINDEX` on Linux, `IP_BOUND_IF` on macOS) and follow Wi-Fi or cable changes; its "system" DNS lookups ask that network's DNS servers directly: the adapter's on Windows; systemd-resolved's or `/etc/resolv.conf`'s on Linux; on macOS the ones DHCP gave (a DNS server set by hand in macOS' settings is not used meanwhile); else the gateway. Linux lets an ordinary program bind a socket to an interface from version 5.7 on: an older Linux has no TUN mode. Only the users who installed the service can use the channel: the pipe's access list keeps others out on Windows; on Linux and macOS the service asks the kernel who connected and answers others `not_allowed`, and the app then installs it for that user too. Both sides learn each other's version first. The TUN lives as long as the core's connection: turning the mode or the proxy off, quitting the app, a crash of the app or the core, signing out, and shutting down remove the adapter, the routes, and the filters within about a second (Windows removes the filters, a dynamic WFP session, with the service's process; on Linux the rules that a crash of the service leaves are removed when it starts again, two seconds later).

The service is installed once, with administrator rights: on Windows by Setup, always, with the service's own administrator prompt (Setup itself runs without administrator rights), or by the app the first time the mode is chosen (a portable copy, or the prompt declined in Setup; one prompt); on Linux (with systemd) and macOS by the app the first time the mode is chosen. **Settings → Advanced** ends with **Remove the TUN service**, red, which asks before it removes the service (choosing TUN again installs it again, with the administrator prompt); Setup's uninstaller removes it on Windows, and `install.sh remove-desktop` on Linux and macOS. `firefox-vpn-tun install | uninstall | update | -version` does the same from a terminal (opened with no command, as from Explorer, it says in a line what it is, with the project's link; on Windows in a console window of its own); on Linux and macOS, `sudo firefox-vpn-tun install` installs it for the user who ran `sudo`. A newer app updates the installed service with no prompt: the service accepts only an executable signed with the key built into it (`firefox-vpn-tun.sig` beside it; in the macOS app in `Contents/Resources`; [Development](DEVELOPMENT.md#signing-key)), and the core asks an older service to update itself before it opens a TUN, and after the app updated itself. A service newer than the app speaks no older protocol; the app then says so.

`GET /api/status` reports `tun_supported` and `tun` ([DASHBOARD.md](DASHBOARD.md#api)): `state` (`off`, `starting`, `on`, `error`), `code` when it does not run (`missing`: not installed, or installed by another user of the computer; `not_running`; `outdated`: older than the app and could not update; `app_outdated`; `busy`: another copy of the app has the TUN; `failed`; `lost`: the adapter went away), `installed`, `service_version`, `programs_supported` (programs can bypass the VPN here), and `refused_udp` (below).

### Programs that bypass the VPN

**Settings → TUN settings** (Windows and Linux) lists the programs whose traffic skips the VPN (`tun.programs`, [CONFIGURATION.md](CONFIGURATION.md#windows-system-proxy)), each with **All traffic** (the whole program connects directly: a game launcher or a torrent client then uses none of an account's 50 GB) or **UDP only** (its UDP alone: a messenger blocked in the country keeps its messages in the VPN while its calls over UDP go the direct way). **Add program** offers three ways: **From running programs** (the programs running now, by the file names the TUN sees, with a search; `GET /api/tun/programs`, which only the desktop app's core answers, without the system's own programs and the app's), **Choose a file…** (the system's open dialog), and **Type a name…**. A program is known by its file name (`steam.exe`, `telegram-desktop`), without case, so it holds after an update moves the program's folder; a name with `*` (any text) or `?` (one character) is a pattern: `steam*` is every Steam program (`steam.exe`, `steamwebhelper.exe`). A name wins over a pattern; patterns are tried in the list's order. The list shows each program's name, and the path it was chosen from (a file, or a running program; none for a name typed by hand), both read left to right from the left in every language; only the name decides, so a program that moved or was updated keeps skipping the VPN, and one deleted or renamed simply matches nothing.

The TUN service finds the program behind each new connection by the connection's address on the adapter: in Windows' tables of TCP and UDP sockets with their processes (`GetExtendedTcpTable`, `GetExtendedUdpTable`), or on Linux in the kernel's (`/proc/net`) and then the process holding the socket. It sends that traffic directly itself, outside the TUN and bound to the physical interface like the core's own (`internal/egress`), a fake address's name resolved with that network's DNS servers; UDP replies reach the app from the address it sent to. QUIC (UDP to port 443) of a program whose UDP alone skips the VPN is still refused, so it falls back to TCP through the VPN. A change of the list applies at once: new TCP connections take it, and every UDP flow is judged again. Limits: the traffic still passes through the service (Windows has no routes per program); the system's own services (Windows Update and the Store, which run in `svchost.exe`), WSL, and containers have no program of their own to choose; macOS does not find connections' programs yet, so it has no such list.

When TUN mode refuses a program's UDP that looks like a call or a game (to an address on the internet, not QUIC, DNS, or the clock; not one of the system's own programs), the service tells the core once per program and TUN session (`udp_refused`, tunapi protocol 2), the status shows it as `tun.refused_udp` (its executable in `tun.refused_udp_path`) while the TUN runs and the program is not in the list, and the home page shows a card: **Send its UDP directly** adds the program with **UDP only**; **Don't show again** turns the card off (Settings' **Notice when UDP is blocked**). The card goes a minute after it can first be seen (the window shown, the app in front), and comes back for another program, or for the same one after the TUN starts again.

## App updates

The desktop app updates itself on Windows (installed by Setup or portable), on Linux (installed by `install.sh desktop`), and on macOS (dragged from the disk image to Applications, or to another folder the user may change). The app looks for a newer release on GitHub (a minute after it starts, then every 5 days; the About page looks at once) and shows it on the home page with **Update**. The release to offer is the one that `.github/updates.json` in the repository names for the app's system (`windows`, `linux`, `macos`, or `android`); the app reads it from GitHub's raw files, through the VPN while it is on. A release that changes nothing for a system leaves that system's older version in the file, so its app does not offer it. The app offers the named version once the release has the system's file (the zip, the tar.gz, or the universal APK; until then it looks again within the hour). When the file cannot be read (some networks block GitHub's raw files) or does not name the system, the app offers the latest release, as versions of the app from before `updates.json` always do. with **Update automatically** (Settings, General; on by default) a version found downloads in the background and installs the next time the app starts, and Update installs it at once. Android shows the download instead.

With each release, `.github/updates.json` gets the release's version for the systems whose users should update; it is read from the default branch, so it can also change later, without a release. CI checks it on every push (an object of those four systems' `major.minor.patch` versions), and on a release warns when it gives the release's version to no system.

The core downloads the release's file for the system, `Firefox-VPN-<version>-Windows-x64.zip`, on Linux `Firefox-VPN-<version>-Linux-<amd64|arm64>.tar.gz` for the processor, and on macOS `Firefox-VPN-<version>-macOS.tar.gz` (the app alone, as the disk image holds it), through the proxy while it is on (GitHub may be blocked), and checks it before anything is installed: `SHA256SUMS.txt` must carry a valid signature of the release key (`SHA256SUMS.txt.sig`, Ed25519; the key built into the core), and the file the SHA-256 that list gives it; only a version newer than the core's is taken. A download that receives nothing for a minute fails, and so does one that does not check out; the app then offers Try again.

To install, the core unpacks the file (a tar.gz's files stay executable or not as they were; hard links, and links that lead outside the app, are refused), copies itself into `update/updater/` in the data folder, starts that copy with `-update`, and the app and the core quit as on Quit (the system proxy is restored, the TUN closes). The copy waits until both have exited and swaps the app's files: each file and folder of the new version replaces the one of the same name (renames on one drive; anything else in the app's folder and `user-data` stay; on macOS the app's folder is its bundle, whose `Contents` is replaced), the replaced ones are kept aside, and on any failure (a file a virus scanner holds, for example) everything is put back. For a copy that Setup installed it also sets the version in Windows' list of installed apps. It then starts the app again, in the tray if it was there. On Linux the copy and the app it starts run in a systemd scope of their own (`systemd-run --user --scope`) when the user's systemd has one to give, so they outlive the unit the old app ran in (KDE starts each app in one, and its end takes its processes along); else in a session of their own. On macOS the copy runs in a session of its own (launchd ends the process group of the app its LaunchAgent started at sign-in), removes the mark of a downloaded app from the bundle (`com.apple.quarantine`, which the Finder's copy from the disk image left and which recorded the user's approval at the first opening; the release's signature vouches for the new version, which macOS then opens without asking again), and opens the app through Launch Services (`open -n`, with `-g` when it starts in the menu bar), as the Finder does; when that fails, it starts the app's executable itself. The new version reports what happened (`update.last` in `GET /api/status`; the home page says it, or a notification when the app is in the tray), removes the old files, turns the proxy on again if it was on, and lets an older TUN service update itself. After an update that did not finish, the old version runs and says why; it does not install again at that start, only at the next one or on Update.

Setup's own silent install is not used for updates: when it fails, Inno Setup's rollback removes the files it added but keeps the ones it already replaced, which can leave half of each version. Setup's uninstaller removes the app's files whichever version put them there. A copy without the release key (a local build without a signing key), a development build (`0.0.0`), and a core that does not run from the app's folder do not update themselves, nor, on Windows, one without `user-data` beside the app, or on Linux and macOS one whose data folder is on another drive than the app or inside it (a portable macOS copy keeps it in the bundle), whose folder the user cannot change (installed by root, or a macOS app opened from its disk image or from where macOS moved it to open it), or that runs as root; the app offers the download. A copy started with `--data-dir=<folder>` starts again with it after the update.

The update keeps its files in `update` in the data folder (`user-data\update\` on Windows, `~/.local/share/firefox-vpn-client/update/` on Linux, `~/Library/Application Support/Firefox VPN Client/update/` on macOS): the downloaded version (`<version>\`, until it is installed or replaced by a newer one), and while installing `new\`, `backup\`, `updater\`, `pending.json`, and `result.json`, all removed at the next start. While the copy installs, it holds a lock on the data folder (a mutex on Windows; `update.lock` in the data folder on Linux and macOS), so a core that uses the same folder and starts meanwhile leaves these files alone; when that lock cannot be taken for another reason, the update goes on without it, and only another running update stops it.

## Sleeping when unused

The proxy starts asleep: starting the program (the tray, the app mode, a terminal copy, or systemd) opens the SOCKS5/HTTP port and the dashboard but sends nothing to Mozilla: no server list, no Firefox version lookup, no proxy pass, no session, no connection test or public IP check, no quota check, and no device registration. The first connection through Mozilla, a connection test, or a real-delay test connects it and waits for that: the server list and a pass (fetched together), the "fastest" measurement when that location is chosen, and the session, then the automatic connection test with the public IP check and the device registration, as a start used to do. The connection that woke it then goes through; it is delayed by that time (a few seconds, longer with "fastest"). Until then the status shows the persisted location for the configured choice, if any. If connecting fails, the proxy stays asleep, `GET /api/status` reports `proxy.wake_error` and `proxy.wake_failed_at`, and connections fail with that error for a while instead of asking Mozilla again: 10 seconds after the first failure, doubling with each further one up to 2 minutes, and 15 minutes (or until the announced time) for a failure retrying cannot fix soon, such as an account that must sign in again, a spent quota, or a refused region. A connection test retries at once, except after such a lasting failure. A network that comes back, a change of accounts, or the app coming to the front (`POST /api/retry`) ends the wait. A connect from the app that does not reach Mozilla within a minute is given up: the proxy goes off again (the phone's VPN too) and the app says why under its button. When the session to Mozilla breaks while the proxy is awake, the proxy reconnects with the pass it has; it asks Mozilla for a new pass only when Mozilla refuses the pass, and then less and less often while refusals go on. When the same Mozilla server fails three times in a row (it cannot be connected to, or its sessions keep breaking), the proxy moves to another server of the same location, in the same city when it has one, skips the failed server for 15 minutes, and remembers the new one for the next start; the log says so (`switching to another server of the location from=... to=...`). Failures of your own network, of the account, or of its proxy pass do not count, and the location never changes: when all of its servers failed, the proxy keeps retrying. When every account turns out to be unusable, the proxy restarts and waits like a start without a usable account.

Settings, Proxy, **Sleep when unused** (`proxy.idle_sleep`: never, 15 minutes (the default), 30 minutes, or 1 hour) saves the accounts when the proxy is not used. After that long without a connection through Mozilla, the proxy closes its session to Mozilla: no proxy pass is renewed and no quota is checked, so the account is not in use. The SOCKS5/HTTP port stays open. The next connection that needs Mozilla wakes it: a new pass and session for the same server (no new selection or delay test), which delays that first connection by a few seconds; a connection test or a real-delay test wakes it too. Direct connections (routing) do not wake it, and a connection that stays open keeps it awake. Meanwhile it does not look connected: Overview's badge, sidebar, and status dot turn indigo and say "Sleeping", the Uptime card becomes **Asleep for** (with the total uptime below), the tray icon is faded, and the tray tooltip and the status screen say "Sleeping".

### Auto-connect at startup

The app's setting "Connect on startup" (off by default; only with "Start on boot") makes the app, started at sign-in, turn the proxy on by itself (`POST /api/power` with `"auto": true`, once per start, when the core answers with the proxy off and an account exists). The core (`Daemon.AutoConnect`, `internal/service/autoconnect.go`) then:

- with `system_proxy.mode` `clear` (proxy only): turns the proxy on, nothing else. Only programs set up to use it use it; it connects to Mozilla when one does and retries by itself after a failure;
- with `set`: turns the proxy on but holds the system proxy back (`auto_connecting` in `GET /api/status`; the app shows Connecting and runs no tests of its own). Once a network is connected it runs the connection test, then again after 10, 20, 40, and 80 seconds (80 from then on), for 5 minutes counted from when the network connected. A passed test lets the system proxy follow. Without one, the proxy is turned off again with no notification, so Windows was never pointed at a proxy that does not work and the user finds the app off. Turning the proxy on or off, or changing the mode, ends the attempt.

## HTTP proxy and the Windows system proxy

The SOCKS5 address also serves HTTP proxy clients (HTTPS through CONNECT, plain `http://` requests forwarded), with the same SOCKS5 username and password (Basic login). The tray and the terminal (also under systemd) run the proxy from their start; the desktop app starts it off, until the user presses its button. On Windows the other choice, as in v2rayN, is what happens to Windows' proxy setting: **Set system proxy** points it at the proxy, so browsers and programs that follow that setting go through the VPN without their own setup; **Clear system proxy** (the default, "Proxy only" in the app) turns Windows' system proxy off once, leaving the VPN to applications set up by hand. The choice is saved (`system_proxy.mode`), so the next start uses it. The tray, the app mode, and the status screen while the proxy runs in its window apply Set only while the proxy runs and restore the previous setting when the proxy stops, the program ends, the tray icon is closed (unless the copy it controls applies the setting itself and keeps running), or the window is closed. The same choice is in the tray menu, Settings, Proxy, the desktop app, and on the status screen.

## Routing (your region's sites directly)

Settings, **Routing**, is off by default. Turned on, the sites and networks of the chosen **Region** (`routing.region`: Iran by default, China, Russia, Afghanistan, Turkey, Indonesia, Brazil, or None), private networks, and the destinations matched by the extra Direct rules go directly through your own internet: faster, not refused for a foreign address, and without using the accounts' quota. The region's rules come from two v2ray lists from Chocolate4U/Iran-v2ray-rules, a domain list (`geosite.dat`) and a network list (`geoip.dat`); they are downloaded on first use and again every 10 days into `user-data/routing`. The region's preset rules (listed in [CONFIGURATION.md](CONFIGURATION.md#routing)) are filled into the Direct rules, where they can be changed or removed; another region replaces them, and **Restore the region's rules** brings them back. Block rules refuse destinations, for example `geosite:ads`. A direct connection shows your real address to that site and is not protected by the VPN. Whatever the rules say, a client on another device (a shared proxy) never reaches this computer itself directly: loopback, unspecified, and link-local addresses (such as a cloud server's metadata service, 169.254.169.254) are refused (`proxy.ErrThisMachine`; HTTP 403 "The proxy does not connect to this computer itself.", SOCKS reply 0x02) and counted as blocked.

## SOCKS5 login

When the proxy listens on the network (for example `0.0.0.0:10810` on a VPS), anyone who finds the port could use it and spend the accounts' quota. Set **SOCKS5 username** and **SOCKS5 password** in Settings (`proxy.username`, `proxy.password`): clients must then log in (SOCKS5 username/password, RFC 1929), and refused clients are logged as warnings. The dashboard warns while the proxy is reachable from the network without them. In clients: `socks5h://user:password@host:10810`, or the SOCKS5 username and password fields of the application. Some browsers do not support SOCKS5 passwords themselves; use a local tool or extension in that case. A forgotten SOCKS5 password can be shown in Settings with the eye button next to it.

## DNS and filtering

The program resolves only its own destinations: Mozilla's Fastly proxy servers (`*.fastly-masque.net`) and the account, Guardian, and server-list services. Websites that applications open through the SOCKS5 proxy are sent by name to Mozilla's exit and resolved there, provided the application lets the proxy resolve names (in browsers: "Proxy DNS when using SOCKS v5", or `socks5h://`).

On a filtered network the system DNS may return wrong addresses for Mozilla's names. **Settings → DNS** chooses other servers: Automatic (Cloudflare, Google, and Quad9 over HTTPS, queried together), one of those providers, Cloudflare over TLS, or a custom list (`https://host/dns-query`, `tls://host`, or an IP address for plain DNS). Answers are cached for their TTL (30 seconds to 10 minutes). By default the system DNS is still used when none of the servers answers. The change applies to new connections at once; the proxy keeps running.

**Test DNS** resolves the four names through the selection and through the system DNS side by side and marks a system answer that is a block-page address (private, loopback, or Iran's filtering page) as **Filtered**: the network tampers with DNS. Answers that merely differ are normal for sites served from many places. Encrypted DNS does not help when the connections themselves are blocked or throttled (by address or by TLS server name); the API relay (`network.api_relay`, below) covers that case for the Mozilla services.

## Location, exit, and delay

### Choosing a location by real delay

**Test real delay** on the dashboard's Servers page and **Choose a location** on the status screen test every location. Results are merged into the earlier ones. For each location the test it opens a session with the active account's proxy pass and times a CONNECT to `www.google.com:443` (the better of two). Every location's server is on Mozilla/Fastly's anycast network, so the connection to the server reaches the same nearby Fastly edge for every location; only a request that travels on to the exit shows the real difference. The proxy must be running. A test takes a few seconds on a normal network and up to two minutes on a slow or filtered one; a location that refuses the account shows "not allowed for this account".

Results are sorted lowest first. **Use** sets that server (Manual); **Auto (fastest)** sets `proxy.country` to `fastest`, which measures at each proxy start (reusing a measurement younger than 30 minutes) and picks the fastest of all locations, including those the server list marks locked; a location that failed the test is not picked. If nothing answers, it falls back to the recommended location.


The configured location comes from Mozilla's server list. Without a country the recommended (anycast) location is used. The **Exit** on the Overview page and the connection test report the address websites actually see, which may be IPv4 or IPv6 and can differ between destinations. Hostname persistence does not pin a physical server or exit IP.

The connection test's **total** includes opening a CONNECT stream, the TLS handshake to the test site, and its first response; it is not ICMP latency.

## Stability and renewal

- The ProxyPass (about ten minutes) renews two minutes before expiry on the existing upstream session; open connections continue.
- Transient Guardian failures retry within the request and again after 30 seconds. Failures that retrying cannot fix soon (sign-in required, region refused, not entitled) back off for 15 minutes; spent quota waits for its reset. The Overview page shows the last renewal error and the next attempt.
- A renewal that exceeds two minutes is cancelled and retried; the process exits only if it cannot unwind within 30 more seconds (systemd restarts it).
- While no network is connected (for example right after the computer starts, before Wi-Fi connects), the proxy does not try Mozilla: the dashboard and the tray say "Waiting for a network connection" (without a notification), and it starts within two seconds of Wi-Fi or a cable connecting. Only the network adapters are looked at (one that is up with a gateway counts; VMware, Hyper-V, and WSL adapters without one do not); a network without internet still counts and the start then retries. This always applies.
- If the proxy cannot start (for example the SOCKS port is in use), the dashboard shows the error and the service retries every 30 seconds (5 seconds right after a network connected).

## Troubleshooting

### Waiting: add an account / no usable account

The proxy starts when at least one enabled account can supply a pass. With accounts present, the Accounts tab shows why each one is not usable: threshold reached or quota used up (until the reset date), needs sign-in, unavailable (rejected, retried after 15 minutes), or disabled.

### Needs sign-in (`reauth_required`)

Mozilla rejected the stored credentials (for example after the account's sessions were revoked). Use **Sign in again** on the account.

### Guardian returns HTTP 451 (`region_unavailable`)

Guardian does not serve the region the API requests come from, or the network blocks Mozilla's sign-in, Guardian, or server list. Set up your own relay and put its URL in **Settings → Advanced → Relay for Mozilla's services** (`network.api_relay`). Only these requests use it (the account sign-in and tokens, Guardian, the server list, the geo files, and flags); the VPN traffic never does.

The relay takes `?url=<target>` and sends the same method, headers, and body to the target; it relays only Mozilla's, GitHub's, and jsDelivr's hosts, so it is not an open proxy, and answers a refusal with an `X-Relay-Error` header, which the program reports. Two ready-made relays are in [scripts/relay](../scripts/relay):

- **Cloudflare Worker** (`worker.js`, free): Cloudflare dashboard → Workers & Pages → Create → Worker, paste the file, set `KEY`, Deploy. The URL is `https://NAME.ACCOUNT.workers.dev/?key=KEY`. Its requests leave from the Cloudflare location nearest you, so it helps against blocking but not always against Guardian's region check.
- **PHP** (`relay.php`, PHP 7.3+ with curl): upload it to a host that serves HTTPS, set `$KEY`; the URL is `https://example.com/relay.php?key=KEY`. Its requests leave from the host's country.

The URL must be `https://`: the sign-in and the account tokens cross the relay, and whoever runs it can read them, so use only a relay you run yourself. Set a `KEY`; without it anyone who finds the URL can use the relay for those hosts.

### Fastly challenge, CAPTCHA, or HTTP 406

Mozilla's account API is behind a Fastly anti-bot check. Proof-of-work challenges are solved automatically; image CAPTCHAs appear in the dashboard sign-in dialog. Background token refreshes cannot show a CAPTCHA; if one blocks them, the account eventually needs a new sign-in.

### A chosen location is refused, or the public IP is in another country

Mozilla's server list marks some locations locked (Firefox shows them only under preconditions), but its proxy servers accept every account on all of them, so the program offers every location. If Mozilla starts refusing a location the user chose, its server answers the CONNECT with 403 or 407 even with a new proxy pass; the connection test then fails with `location_refused` ("Mozilla does not let this account use this location"), and the app offers a button to switch to the recommended location. Choose another location or the recommended one.

When the public IP's country (from the exit probe) is not the country chosen, `GET /status` reports `exit_mismatch` and the app shows a soft warning; the connection still works. Not reported for the recommended location or Auto (fastest).

### A site on an unusual port does not open

Mozilla's proxy connects only to the web ports **80, 443, 8080, and 8443**; for every other port (for example `http://203.0.113.5:9898/` or SSH on 22) it answers `502 Bad Gateway`. This is Mozilla's rule for its browser VPN and cannot be changed by this program. The log says `Mozilla's proxy does not connect to port …`, and SOCKS clients get the reply "connection not allowed by ruleset"; the dashboard shows a note on Overview and explains it under **Help**. Serve the site on one of the four ports, or let the browser reach it without the proxy (Firefox: Settings, Network Settings, **No proxy for**). Measured on 2026-10-10 through the tunnel.

DNS on port 53 is the exception: a client's DNS sent through the proxy over TCP (PassWall2's remote DNS, v2rayN's or Clash's DNS through a SOCKS outbound) is answered by this program itself, each message as it is with DNS over HTTPS through Mozilla (1.1.1.1 and 8.8.8.8, whichever answers first), SERVFAIL when neither does (`internal/proxy/dns.go`). A routing rule that sends the DNS server directly still does. DNS over UDP needs SOCKS5's UDP ASSOCIATE, which this program does not have.

### Nothing connects: a loop

When another program sends this program's own traffic into its proxy (PassWall2's Localhost Proxy on a router, another VPN program's TUN mode without an exception for `firefox-vpn-core`), the proxy needs those very connections to reach Mozilla, and nothing connects. The program notes the addresses of its own connections for 10 seconds (`egress.OwnDial`; not loopback); when two different ones come back as clients' connections within that time, the log says `a loop: this program's own connections come back to its proxy …` (once a minute) and the dashboard shows a note. Turn Localhost Proxy off, or let `firefox-vpn-core` connect directly in the other program.

### Port already in use

The proxy or dashboard address is taken by another program (or another copy of this one). Change `proxy.listen` or `dashboard.listen`, or stop the other program. Do not stop unrelated software to free a port. While the proxy address is taken, the status shows the error with the address (`state.code` `proxy_port_in_use` in `GET /api/status`) and the start is retried every 30 seconds; a new `proxy.listen` saved in Settings applies at once. The desktop app's core process exits with status 4 and an `app-error: dashboard_port_in_use` line when the dashboard address is taken ([App error line](#app-error-line)).

### Log files

The log is written to `user-data/logs`, one file per day (`firefox-vpn-2026-10-10.log`, by local date). The last seven days are kept: when the eighth day starts, the oldest file is deleted. Ordinary logging writes a few kilobytes a day; as a safety limit a day's file stops at 10 MB (only verbose logging under heavy traffic gets there) with a note, and logging continues in the next day's file. Several processes (a tray and a status screen) can write the same file. When the day's file is deleted while the program runs, it is created again within ten seconds.

### The tray icon is grey

The tray cannot reach the service: it is stopped or still starting, the dashboard is disabled in `config.json` (`dashboard.enabled`), or `user-data\local-api.token` is unreadable. The tooltip reads "Service not reachable" or "Starting…". When the tray's own proxy cannot start, for example because a value in `config.json` is invalid or another program holds `dashboard.listen`, the tooltip adds "Could not start: …" (or "Stopped: …") with the error, the day's log in `user-data\logs` has the whole message, and the tray tries again about once a minute. An older tray that saw a newer version serve the folder never starts the proxy itself; it closes when that copy stops (see [Version](#version)).

### HTTP/3 times out

Live HTTP/3 has not been verified to work with Mozilla's proxies; UDP may also be blocked on the network. Use HTTP/2.

## Credentials and limits

- Per account, `user-data/accounts/<id>/credentials.json` holds the OAuth access token (about one day), the refresh token for the scope `profile https://identity.mozilla.com/apps/vpn`, and the Firefox Account password, encrypted with a key kept in `user-data/accounts/secret.key`. The password lets the program sign in again by itself when Mozilla revokes the tokens. The encryption only keeps the password out of plain sight: anyone with the whole data folder can recover it, so keep the folder private and never share it. Sign-in itself still sends only `authPW`, derived from the password locally as Firefox does.
- Each account appears in its Mozilla account's connected services as **Firefox's built-in VPN**. Tokens can be revoked there; the program then signs in again with the stored password, or asks for **Sign in again** when Mozilla wants a confirmation code.
- On Windows the files take the data folder's normal permissions; on Linux they are `0600`.
- The dashboard has no HTTPS.
- Account eligibility, server availability, and quotas are decided by Mozilla. Respect the service's terms and quotas.
