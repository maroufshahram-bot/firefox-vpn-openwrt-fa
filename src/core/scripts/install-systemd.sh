#!/usr/bin/env bash
# Installs firefox-vpn-core as a systemd service on Linux, from the source
# tree or from a prebuilt binary. The release's install.sh (at the repository
# root) runs it with the downloaded binary and keeps it as the firefox-vpn
# command.
#
# The program and its data folder live together in INSTALL_DIR (portable
# layout): INSTALL_DIR/firefox-vpn-core and INSTALL_DIR/user-data. Settings and
# accounts are managed from the web dashboard (default 127.0.0.1:10820) or
# with the subcommands below.
set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-firefox-vpn}"
SERVICE_USER="${SERVICE_USER:-firefox-vpn}"
INSTALL_DIR="${INSTALL_DIR:-/opt/firefox-vpn}"
BINARY="${BINARY:-}"
PURGE="${PURGE:-0}"
# How the help and the messages name this script (install.sh: firefox-vpn).
SELF="${SELF:-$0}"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BIN_PATH="$INSTALL_DIR/firefox-vpn-core"
DATA_DIR="$INSTALL_DIR/user-data"
# update runs the release's installer, which downloads the newest core,
# keeps the data folder (accounts, settings, logins), and replaces this
# command too; RELEASES_URL/latest leads to the newest release's tag.
INSTALLER_URL="${INSTALLER_URL:-https://raw.githubusercontent.com/armanlife/firefox-vpn-client/main/install.sh}"
RELEASES_URL="${RELEASES_URL:-https://github.com/armanlife/firefox-vpn-client/releases}"

# The command being run, for the messages.
COMMAND_NAME="${1:-}"
# The name people type (firefox-vpn), not the path.
NAME="$(basename "$SELF")"
# Colors on a terminal (unless NO_COLOR is set): errors in red, the menu.
if [ -t 2 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-}" != dumb ]; then
  RED=$'\e[1;31m' OFF=$'\e[0m' BOLD=$'\e[1m' DIM=$'\e[2m' CYAN=$'\e[36m' FOX=$'\e[38;5;208m' GREEN=$'\e[32m'
else
  RED="" OFF="" BOLD="" DIM="" CYAN="" FOX="" GREEN=""
fi

# done_ok, done_fail: what a command did, in green or red.
done_ok() { printf '  %s✓%s %s\n' "$GREEN" "$OFF" "$*"; }
done_fail() { printf '  %s✗ %s%s\n' "$RED" "$*" "$OFF" >&2; return 1; }

log() { printf '[%s] %s\n' "$SERVICE_NAME" "$*"; }
die() { printf '[%s] error: %s\n' "$SERVICE_NAME" "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: sudo $NAME [command]

Without a command, on a terminal, a menu offers them by number.

Commands:
  install              Install or update the binary and the service, then start it
  update               Update to the newest release, keeping the accounts and settings
  uninstall            Remove the service, and this command; on a terminal it asks first
                       and whether to delete $INSTALL_DIR (else PURGE=1 does)
  status               Show the service state and the runtime status file
  start                Start the service
  stop                 Stop the service (until start, or the next boot when enabled)
  restart              Restart the service
  enable               Start the service with the server
  disable              Do not start the service with the server
  logs                 Show the service log
  add-account          Sign in to a Firefox Account on this terminal
  list-accounts        List accounts, status, and quota
  panel                Set the panel's username and password (random when left empty)
  panel-info           Show the panel's addresses and username
  setup-server [user]  Open the dashboard and the proxy to the network: the dashboard
                       login for user (a random one without it) and a random proxy
                       login, each with a random password; print the logins

Environment:
  BINARY=path          Install this prebuilt binary instead of building from the source tree
  INSTALL_DIR=dir      Program and data folder (default /opt/firefox-vpn)
  SERVICE_NAME, SERVICE_USER

The dashboard is at http://<address>:10820/app/. It opens to other devices only
with a login (panel, or setup-server, which install.sh runs on a
new server); without one it answers this machine only, and an SSH tunnel
(ssh -L 10820:127.0.0.1:10820 host) reaches it.
EOF
}

# need_root: stops, in red, when the command needs root and was run
# without it.
need_root() {
  [ "$(id -u)" -ne 0 ] || return 0
  printf '%sPermission denied: this needs root. Run it with sudo:%s\n  sudo %s %s\n' \
    "$RED" "$OFF" "$(basename "$SELF")" "$COMMAND_NAME" >&2
  exit 1
}

repo_root() { cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd; }

build_binary() {
  local out="$1"
  if [ -n "$BINARY" ]; then
    [ -f "$BINARY" ] || die "BINARY=$BINARY does not exist"
    cp "$BINARY" "$out"
    return
  fi
  # Installed as the firefox-vpn command (install.sh), with no source tree.
  [ -d "$(repo_root)/cmd/firefox-vpn-core" ] ||
    die "no source tree to build from; to update, run: sudo $NAME update"
  command -v go >/dev/null 2>&1 || die "Go is not installed; install Go or pass BINARY=path/to/firefox-vpn-core"
  log "building from $(repo_root)"
  (cd "$(repo_root)" && CGO_ENABLED=0 go build -trimpath -o "$out" ./cmd/firefox-vpn-core)
}

ensure_user() {
  if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    log "creating system user $SERVICE_USER"
    useradd --system --home-dir "$DATA_DIR" --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
  fi
}


write_unit() {
  cat >"$UNIT_FILE" <<EOF
[Unit]
Description=Firefox VPN SOCKS5 proxy and dashboard
After=network-online.target
Wants=network-online.target

[Service]
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$BIN_PATH
Restart=on-failure
RestartSec=10
UMask=0077
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF
}

run_as_service_user() {
  [ -x "$BIN_PATH" ] || die "not installed; run: sudo $SELF install"
  runuser -u "$SERVICE_USER" -- "$BIN_PATH" "$@"
}

cmd_install() {
  need_root
  command -v systemctl >/dev/null 2>&1 || die "systemd is required"
  local tmp
  tmp="$(mktemp)"
  # The value now: the local variable is gone when the script exits.
  trap "rm -f -- '$tmp'" EXIT
  build_binary "$tmp"
  ensure_user
  install -d -m 0755 -o root -g root "$INSTALL_DIR"
  install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_USER" "$DATA_DIR"
  install -m 0755 -o root -g root "$tmp" "$BIN_PATH"
  write_unit
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}.service" >/dev/null
  systemctl restart "${SERVICE_NAME}.service"
  log "installed $BIN_PATH; data in $DATA_DIR"
  log "dashboard: http://127.0.0.1:10820/app/ (add accounts there, or: sudo $SELF add-account)"
}

# latest_version: the newest release's version (1.2.3), from where its
# latest link leads; empty when GitHub cannot be reached.
latest_version() {
  local url=""
  if command -v curl >/dev/null 2>&1; then
    url="$(curl -fsSI -o /dev/null -w '%{redirect_url}' --max-time 20 "$RELEASES_URL/latest" 2>/dev/null || true)"
  elif command -v wget >/dev/null 2>&1; then
    # wget names the address twice, the second time with " [following]".
    url="$(wget -S --spider --max-redirect=0 -T 20 "$RELEASES_URL/latest" 2>&1 |
      sed -n 's/^ *[Ll]ocation: *\([^ ]*\).*/\1/p' | tr -d '\r' | head -n 1 || true)"
  fi
  case "$url" in
    */tag/v*) printf '%s\n' "${url##*/tag/v}" ;;
  esac
}

# newer <a> <b>: whether version a (major.minor.patch) is newer than b.
newer() {
  [ "$1" != "$2" ] && [ "$(printf '%s\n' "$1" "$2" | sort -V | tail -n 1)" = "$1" ]
}

# cmd_update: the newest release, through its installer, unless the one
# installed is as new. Failures return: in the menu, errexit is off.
cmd_update() {
  need_root
  [ -x "$BIN_PATH" ] || die "not installed; install it with: curl -fsSL $INSTALLER_URL | sudo bash"
  local current latest script
  current="$("$BIN_PATH" -version 2>/dev/null || true)"
  latest="$(latest_version)"
  if [ -n "$latest" ] && ! newer "$latest" "$current"; then
    done_ok "Already the newest version ($current)."
    return 0
  fi
  if [ -n "$latest" ]; then
    printf '  Updating %s to %s\n' "${current:-the installed version}" "$latest"
  else
    printf '  GitHub did not say which version is the newest; updating anyway\n'
  fi
  if command -v curl >/dev/null 2>&1; then
    script="$(curl -fsSL --retry 3 "$INSTALLER_URL")" || script=""
  elif command -v wget >/dev/null 2>&1; then
    script="$(wget -qO- "$INSTALLER_URL")" || script=""
  fi
  [ -n "$script" ] || { done_fail "Could not download the installer: $INSTALLER_URL"; return 1; }
  local -a vars=(INSTALL_DIR="$INSTALL_DIR" SERVICE_NAME="$SERVICE_NAME" SERVICE_USER="$SERVICE_USER")
  [ -z "$latest" ] || vars+=(VERSION="v$latest")
  env "${vars[@]}" bash -c "$script" install.sh server
}

# cmd_uninstall removes the service. On a terminal it asks first, and
# whether to delete the accounts and settings too (else PURGE=1 does); the
# firefox-vpn command that install.sh made goes with it.
cmd_uninstall() {
  need_root
  if [ -t 0 ] && [ -t 1 ]; then
    local reply
    printf '  Remove Firefox VPN from this server? [y/N] '
    read -r reply || reply=""
    case "$reply" in [yY]*) ;; *) printf '  Nothing removed.\n'; return 0 ;; esac
    printf '  Delete the accounts and settings too? [y/N] '
    read -r reply || reply=""
    case "$reply" in [yY]*) PURGE=1 ;; *) PURGE=0 ;; esac
  fi
  systemctl disable --now "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
  rm -f "$UNIT_FILE"
  systemctl daemon-reload
  if [ "$PURGE" = "1" ]; then
    rm -rf "$INSTALL_DIR"
    userdel "$SERVICE_USER" >/dev/null 2>&1 || true
  fi
  if [ "$SELF" = /usr/local/bin/firefox-vpn ]; then
    rm -f "$SELF"
  fi
  if [ "$PURGE" = "1" ]; then
    done_ok "Firefox VPN is removed, with its accounts and settings."
  else
    done_ok "Firefox VPN is removed; its accounts and settings stay in $DATA_DIR."
  fi
}

# cmd_status: a few lines: the service and its autostart from systemd, then
# the core's own (proxy, account, data left). systemctl status
# firefox-vpn has every detail.
cmd_status() {
  local active since auto=off
  active="$(systemctl is-active "${SERVICE_NAME}.service" 2>/dev/null || true)"
  [ "$(systemctl is-enabled "${SERVICE_NAME}.service" 2>/dev/null || true)" != enabled ] || auto=on
  printf '\n  %s%sFirefox VPN%s\n' "$BOLD" "$FOX" "$OFF"
  printf '  %s──────────────────────────────────────────────────────%s\n' "$DIM" "$OFF"
  if [ "$active" = active ]; then
    since="$(systemctl show -p ActiveEnterTimestamp --value "${SERVICE_NAME}.service" 2>/dev/null || true)"
    since="$(date -d "$since" '+%b %-d %H:%M' 2>/dev/null || true)"
    printf '    %-11s %s●%s %srunning%s       %s\n' "Service" "$GREEN" "$OFF" "$BOLD" "$OFF" "${since:+since $since}"
  else
    printf '    %-11s %s●%s %sstopped%s       start it: %s start\n' "Service" "$RED" "$OFF" "$BOLD" "$OFF" "$NAME"
  fi
  printf '    %-11s %s\n' "Autostart" "$auto"
  if [ "$active" = active ]; then
    if [ "$(id -u)" -eq 0 ] && [ -x "$BIN_PATH" ]; then
      run_as_service_user -status 2>/dev/null || true
    else
      printf '    %s(the account and data left need root: sudo %s status)%s\n' "$DIM" "$NAME" "$OFF"
    fi
  fi
}

# dashboard_field <name> <config.json>: a field of the "dashboard" settings
# (the proxy has a listen and a username of its own).
dashboard_field() {
  awk -v key="\"$1\":" '/"dashboard": *\{/ { d = 1 } d && index($0, key) { sub(/^[^:]*: *"/, ""); sub(/".*/, ""); print; exit }' "$2"
}

# public_ip: the address the Internet sees, from the first of several
# services that answers (one may be down or filtered), or nothing.
public_ip() {
  local ip url
  for url in https://api.ipify.org https://checkip.amazonaws.com https://ipv4.icanhazip.com https://api.ip.sb/ip; do
    ip="$(curl -fsS -4 --max-time 3 "$url" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      echo "$ip"
      return 0
    fi
  done
}

# cmd_panel_info: where the panel opens, and its username. Its password is
# kept only as a hash, so it cannot be shown: "panel" sets a new one.
cmd_panel_info() {
  need_root
  local cfg="$DATA_DIR/config.json" listen user port host lan wan
  [ -f "$cfg" ] || done_fail "No settings yet: $cfg" || return
  listen="$(dashboard_field listen "$cfg")"
  user="$(dashboard_field username "$cfg")"
  port="${listen##*:}"
  host="${listen%:*}"
  printf '\n  %s%sFirefox VPN panel%s\n' "$BOLD" "$FOX" "$OFF"
  printf '  %s──────────────────────────────────────────────────────%s\n' "$DIM" "$OFF"
  case "$host" in
    127.* | localhost | "[::1]")
      printf '    %-11s %shttp://%s/app/%s  %s(this server only)%s\n' "Address" "$CYAN" "$listen" "$OFF" "$DIM" "$OFF"
      ;;
    *)
      lan="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit } }')"
      wan="$(public_ip)"
      [ "$lan" != "$wan" ] || lan=""
      [ -z "$lan" ] || printf '    %-11s %shttp://%s:%s/app/%s\n' "Local" "$CYAN" "$lan" "$port" "$OFF"
      [ -z "$wan" ] || printf '    %-11s %shttp://%s:%s/app/%s\n' "Global" "$CYAN" "$wan" "$port" "$OFF"
      ;;
  esac
  if [ -n "$user" ]; then
    printf '    %-11s %s%s%s\n' "Username" "$BOLD" "$user" "$OFF"
  else
    printf '    %-11s none: set one with %s panel\n' "Username" "$NAME"
  fi
}

# menu: the everyday commands by number, for the command without
# arguments on a terminal; it runs the one picked.
menu() {
  local reply
  # The commands in groups, "-" between them (a rule in the menu):
  # updating and removing, the panel, the service and its log, starting
  # with the server, the accounts. Numbers count the commands only.
  local -a items=(
    "update|Update"
    "uninstall|Uninstall"
    -
    "panel|Set panel login"
    "panel-info|View panel info"
    -
    "status|Status"
    "start|Start"
    "stop|Stop"
    "restart|Restart"
    "logs|Show the log"
    -
    "enable|Enable autostart"
    "disable|Disable autostart"
    -
    "add-account|Add a Firefox Account"
    "list-accounts|List the accounts"
  )
  local -a cmds=()
  local item rule
  rule="$(printf '  %s────────────────────────────────────────────────────────────────────────────%s' "$DIM" "$OFF")"
  # Back to the menu after each command, until 0.
  while :; do
    cmds=()
    printf '\n  %s%sFirefox VPN%s\n' "$BOLD" "$FOX" "$OFF"
    printf '%s\n' "$rule"
    printf '  %s 0)%s Exit\n' "$CYAN" "$OFF"
    printf '%s\n' "$rule"
    for item in "${items[@]}"; do
      if [ "$item" = - ]; then
        printf '%s\n' "$rule"
        continue
      fi
      cmds+=("${item%%|*}")
      printf '  %s%2d)%s %-40s %s%s %s%s\n' "$CYAN" "${#cmds[@]}" "$OFF" "${item#*|}" "$DIM" "$NAME" "${item%%|*}" "$OFF"
    done
    printf '%s\n' "$rule"
    while :; do
      printf '  Enter a number [0-%d]: ' "${#cmds[@]}"
      read -r reply || exit 0
      case "$reply" in
        0 | "") exit 0 ;;
        *[!0-9]*) ;;
        *) if [ "$reply" -ge 1 ] && [ "$reply" -le "${#cmds[@]}" ]; then break; fi ;;
      esac
      printf '  %sNo such choice: %s%s\n' "$RED" "$reply" "$OFF"
    done
    COMMAND_NAME="${cmds[$((reply - 1))]}"
    printf '\n'
    # Ctrl+C ends the command (the log, for one), not the menu.
    trap ':' INT
    ( main "$COMMAND_NAME" ) || true
    trap - INT
    # Removed: nothing is left to manage.
    if [ "$COMMAND_NAME" = uninstall ] && [ ! -f "$UNIT_FILE" ]; then
      exit 0
    fi
    # A blank line below the prompt too, so it never sits on the window's
    # bottom edge: print it, then move the cursor back up onto the prompt.
    printf '\n\n\033[1A  %s↵  Press Enter to return to the menu%s ' "$CYAN" "$OFF"
    read -r reply || exit 0
  done
}

main() {
case "${1:-}" in
  install) cmd_install ;;
  update) cmd_update ;;
  uninstall) cmd_uninstall ;;
  status) cmd_status ;;
  logs) journalctl -u "${SERVICE_NAME}.service" -f ;;
  start)
    need_root
    if systemctl is-active --quiet "${SERVICE_NAME}.service"; then done_ok "Already running."
    else systemctl start "${SERVICE_NAME}.service" && done_ok "Started." || done_fail "Starting failed: $NAME logs"; fi
    ;;
  stop) need_root; systemctl stop "${SERVICE_NAME}.service" && done_ok "Stopped." || done_fail "Stopping failed" ;;
  restart) need_root; systemctl restart "${SERVICE_NAME}.service" && done_ok "Restarted." || done_fail "Restarting failed: $NAME logs" ;;
  enable) need_root; systemctl enable --quiet "${SERVICE_NAME}.service" && done_ok "Starts with the server." || done_fail "Failed" ;;
  disable) need_root; systemctl disable --quiet "${SERVICE_NAME}.service" && done_ok "Does not start with the server." || done_fail "Failed" ;;
  add-account) need_root; run_as_service_user -add-account && systemctl restart "${SERVICE_NAME}.service" ;;
  list-accounts) need_root; run_as_service_user -list-accounts ;;
  panel) need_root; run_as_service_user -panel && systemctl restart "${SERVICE_NAME}.service" ;;
  panel-info) cmd_panel_info ;;
  setup-server) need_root; run_as_service_user -setup-server "${2:--}" && systemctl restart "${SERVICE_NAME}.service" ;;
  "")
    # On a terminal, the menu; otherwise (scripts, pipes) the help.
    if [ -t 0 ] && [ -t 1 ]; then menu; else usage; fi
    ;;
  -h|--help|help) usage ;;
  *) usage; exit 2 ;;
esac
}

main "$@"
