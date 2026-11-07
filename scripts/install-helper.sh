#!/usr/bin/env bash
# Install (or remove) the privileged helper's systemd units: the operator's first gate (ADR-0034, Constitution I.11). The service can never run this.
#
#   sudo scripts/install-helper.sh net --install --actions scan_udp,os_detect --scope 192.168.50.0/24 --bin ./llm-proxy-helper
#   sudo scripts/install-helper.sh net --uninstall
#
# What it does, in order, stopping at the first problem: checks the host (Linux with systemd, ptrace_scope not 0, the service's group exists,
# nmap is installed), installs the binary root-owned, renders the units from docs/services/, writes the root-owned drop-in with the two settings
# the service cannot change (HELPER_ACTIONS, HELPER_SCOPE), reloads systemd and starts the socket. Nothing runs as root until the socket is
# used, and then one short-lived process per connection. Re-running it replaces the units and the drop-in.
#   --dry-run   print what would be written and run nothing
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DOCS="$HERE/../docs/services"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"
CLASS="" MODE="" ACTIONS="" SCOPE="" BIN="" GROUP="llm-proxy" SERVICE_UNIT="llm-proxy.service" DRY=0
HELPER_BIN="${HELPER_BIN:-/usr/local/bin/llm-proxy-helper}"

die() { printf 'install-helper: %s\n' "$*" >&2; exit 1; }
say() { printf '  %s\n' "$*"; }

[[ $# -ge 1 ]] || die "usage: install-helper.sh net --install --actions a,b --scope CIDR,CIDR --bin PATH | net --uninstall"
CLASS="$1"; shift
[[ "$CLASS" == "net" ]] || die "the class must be net (the log class is deferred)"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install) MODE=install; shift ;;
    --uninstall) MODE=uninstall; shift ;;
    --actions) ACTIONS="${2:?--actions needs a value}"; shift 2 ;;
    --scope) SCOPE="${2:?--scope needs a value}"; shift 2 ;;
    --bin) BIN="${2:?--bin needs a path}"; shift 2 ;;
    --group) GROUP="${2:?--group needs a name}"; shift 2 ;;
    --service-unit) SERVICE_UNIT="${2:?--service-unit needs a name}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    *) die "unknown argument: $1" ;;
  esac
done
[[ -n "$MODE" ]] || die "give --install or --uninstall"

SOCKET_UNIT="llm-proxy-helper-$CLASS.socket"
SERVICE_TPL="llm-proxy-helper-$CLASS@.service"
DROPIN_DIR="$UNIT_DIR/$SERVICE_TPL.d"

run() { if [[ $DRY -eq 1 ]]; then printf '  would run: %s\n' "$*"; else "$@"; fi; }

uninstall() {
  [[ $DRY -eq 1 || "$(id -u)" -eq 0 ]] || die "run as root (sudo)"
  run systemctl disable --now "$SOCKET_UNIT" 2>/dev/null || true
  run rm -rf "$DROPIN_DIR" "$UNIT_DIR/$SOCKET_UNIT" "$UNIT_DIR/$SERVICE_TPL" "$HELPER_BIN"
  run systemctl daemon-reload
  say "removed: the helper's units, drop-in and binary. The service keeps working; its privileged actions now report unavailable."
}

# validate_list checks a comma-separated list against a pattern, so nothing but what is expected can reach a unit file.
validate_list() { # name value pattern
  [[ -n "$2" ]] || die "$1 is required"
  [[ "$2" =~ ^$3(,$3)*$ ]] || die "$1 has a value that is not allowed: $2"
}

install_checks() {
  [[ "$(uname -s)" == "Linux" ]] || die "the helper is Linux only (macOS reports the actions unavailable)"
  [[ $DRY -eq 1 || "$(id -u)" -eq 0 ]] || die "run as root (sudo)"
  command -v systemctl >/dev/null || die "systemd is required"
  validate_list --actions "$ACTIONS" '[a-z][a-z0-9_]{0,31}'
  validate_list --scope "$SCOPE" '[0-9]{1,3}(\.[0-9]{1,3}){3}/[0-9]{1,2}'
  [[ -n "$BIN" && -f "$BIN" ]] || die "--bin must be the built helper (cd backend && go build -o llm-proxy-helper ./cmd/llm-proxy-helper)"
  getent group "$GROUP" >/dev/null || die "the group '$GROUP' does not exist: it is the service's group, the only one allowed to open the socket"
  command -v nmap >/dev/null || die "nmap is not installed (sudo apt install nmap)"
  local scope_file=/proc/sys/kernel/yama/ptrace_scope
  [[ -r "$scope_file" ]] || die "Yama ptrace_scope is not available: without it a child of the service could read the service's memory, and the helper refuses to run"
  [[ "$(cat "$scope_file")" != "0" ]] || die "ptrace_scope is 0 (sudo sysctl kernel.yama.ptrace_scope=1): the helper refuses to run while a process can read another's memory"
}

render() { # template
  sed -e "s|@SERVICE_GROUP@|$GROUP|g" -e "s|@HELPER_BIN@|$HELPER_BIN|g" -e "s|@SERVICE_UNIT@|$SERVICE_UNIT|g" "$1"
}

install_all() {
  install_checks
  say "installing the $CLASS class: actions=$ACTIONS scope=$SCOPE group=$GROUP service=$SERVICE_UNIT"
  if [[ $DRY -eq 1 ]]; then
    render "$DOCS/$SOCKET_UNIT"; render "$DOCS/$SERVICE_TPL"
    printf '[Service]\nEnvironment=HELPER_ACTIONS=%s\nEnvironment=HELPER_SCOPE=%s\n' "$ACTIONS" "$SCOPE"
    return
  fi
  getent passwd llm-proxy-helper >/dev/null ||
    useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin --user-group llm-proxy-helper
  install -o root -g root -m 0755 "$BIN" "$HELPER_BIN"
  render "$DOCS/$SOCKET_UNIT" | install -o root -g root -m 0644 /dev/stdin "$UNIT_DIR/$SOCKET_UNIT"
  render "$DOCS/$SERVICE_TPL" | install -o root -g root -m 0644 /dev/stdin "$UNIT_DIR/$SERVICE_TPL"
  install -d -o root -g root -m 0755 "$DROPIN_DIR"
  printf '[Service]\nEnvironment=HELPER_ACTIONS=%s\nEnvironment=HELPER_SCOPE=%s\n' "$ACTIONS" "$SCOPE" |
    install -o root -g root -m 0644 /dev/stdin "$DROPIN_DIR/10-gates.conf"
  systemctl daemon-reload
  systemctl enable --now "$SOCKET_UNIT"
  systemctl is-active --quiet "$SOCKET_UNIT" || die "the socket did not start: systemctl status $SOCKET_UNIT"
  say "the socket is listening. It does nothing until the service calls it, and refuses every caller but the service's main process."
  say "Next: turn the actions on in Settings (they need your passphrase), then check from the assistant's terminal that it is refused:"
  say "  python3 -c \"import socket; s=socket.socket(socket.AF_UNIX); s.connect('/run/llm-proxy-helper-$CLASS.sock')\"   (expect: refused)"
}

case "$MODE" in
  install) install_all ;;
  uninstall) uninstall ;;
esac
