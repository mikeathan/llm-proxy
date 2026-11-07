#!/usr/bin/env bash
# The helper's acceptance check under real systemd (ADR-0034): installs the helper's units for a throw-away test "service", then calls it from
# that service's main process and from a child of it, and prints what each got. Run as root on Linux with systemd and nmap:
#
#   sudo scripts/check-helper-under-unit.sh --bin ./llm-proxy-helper --target 192.168.50.1
#
# Expect: A ok with real nmap output; B and C refused "scope"; D refused "disabled"; E (a child of the main process, same account and cgroup)
# refused "caller". Then the helper's units, binary and the test account are removed. It refuses to run when the helper is already installed (it
# would replace your real units); uninstall first with scripts/install-helper.sh net --uninstall. Only the target gets packets: two UDP ports.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
BIN="" TARGET=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin) BIN="${2:?}"; shift 2 ;;
    --target) TARGET="${2:?}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$BIN" && -f "$BIN" && -n "$TARGET" ]] || { echo "usage: check-helper-under-unit.sh --bin ./llm-proxy-helper --target <private IPv4 of one device of yours>" >&2; exit 2; }
[[ "$(id -u)" -eq 0 ]] || { echo "run as root (sudo)" >&2; exit 2; }
[[ "$TARGET" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || { echo "--target must be one IPv4 address" >&2; exit 2; }
if systemctl cat llm-proxy-helper-net.socket >/dev/null 2>&1; then
  echo "the helper is already installed here; this check would replace its units. Run: scripts/install-helper.sh net --uninstall" >&2; exit 2
fi

WORK="$(mktemp -d /tmp/helper-check.XXXXXX)"; chmod 755 "$WORK"
GROUP=llmpxcheck USERN=llmpxcheck UNIT=llm-proxy-check.service
cleanup() {
  systemctl stop "$UNIT" 2>/dev/null || true
  "$HERE/install-helper.sh" net --uninstall >/dev/null 2>&1 || true
  userdel "$USERN" 2>/dev/null || true; groupdel "$GROUP" 2>/dev/null || true
  userdel llm-proxy-helper 2>/dev/null || true; groupdel llm-proxy-helper 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

cat >"$WORK/client.py" <<'PY'
import base64, json, os, socket, struct, subprocess, sys
SOCK = "/run/llm-proxy-helper-net.sock"
def call(body):
    s = socket.socket(socket.AF_UNIX); s.settimeout(120)
    try: s.connect(SOCK)
    except OSError as e: return "connect refused: %s" % e
    raw = json.dumps(body).encode(); s.sendall(struct.pack(">I", len(raw)) + raw)
    hdr = b""
    while len(hdr) < 4:
        c = s.recv(4 - len(hdr))
        if not c: return "closed with no answer"
        hdr += c
    n = struct.unpack(">I", hdr)[0]; data = b""
    while len(data) < n:
        c = s.recv(n - len(data))
        if not c: break
        data += c
    r = json.loads(data)
    if r.get("result"): r["result"]["data"] = base64.b64decode(r["result"]["data"]).decode(errors="replace")[:300]
    return r
target = sys.argv[2]
req = lambda t: {"v": 1, "id": "t1", "action": "scan_udp", "params": {"target": t, "ports": [53, 161]}}
if sys.argv[1] == "main":
    print("A. main process, target in scope  :", json.dumps(call(req(target))))
    print("B. main process, a public target  :", json.dumps(call(req("8.8.8.8"))))
    print("C. main process, outside the scope:", json.dumps(call(req("192.168.77.5"))))
    print("D. main process, action switched off:", json.dumps(call({"v": 1, "id": "t1", "action": "os_detect", "params": {"target": target}})))
    print("E. a child of the main process    :", subprocess.run([sys.executable, __file__, "child", target], capture_output=True, text=True).stdout.strip())
else:
    print(json.dumps(call(req(target))))
PY
cp "$BIN" "$WORK/helper"; chmod 755 "$WORK/helper" "$WORK/client.py"
groupadd -r "$GROUP"; useradd -r -g "$GROUP" -s /usr/sbin/nologin "$USERN"
"$HERE/install-helper.sh" net --install --actions scan_udp --scope "$TARGET/32" --bin "$WORK/helper" --group "$GROUP" --service-unit "$UNIT" >/dev/null
systemd-run --unit "${UNIT%.service}" --uid "$USERN" --gid "$GROUP" --wait --pipe --quiet -p KillMode=control-group python3 "$WORK/client.py" main "$TARGET"
echo; echo "journal (what the helper logged):"; journalctl --no-pager -o cat -t llm-proxy-helper -u 'llm-proxy-helper-net@*' --since "-2min" 2>/dev/null | grep -E "level=|refused" | tail -5 || true
