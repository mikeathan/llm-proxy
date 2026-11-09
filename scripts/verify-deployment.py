#!/usr/bin/env python3
"""Read-only check of a deployed llm-proxy service: does it run the version you expect, are its parts working, and did the
nightly Network findings change since the last night?

Only GET requests are sent, so it is safe against a live server. Needs the admin API token of the server:

    export LLM_PROXY_TOKEN=...            # Settings > Security > Script access (admin token)
    scripts/verify-deployment.py --base http://vertex.local:4001
    scripts/verify-deployment.py --base ... --workspace-id 1 --automation network-audit --save-baseline night1.json
    scripts/verify-deployment.py --base ... --workspace-id 1 --automation network-audit --compare night1.json --json

Each check is PASS, WARN (worth a look), FAIL (something is wrong), or SKIP (not switched on, or not asked for). Nothing is
reported as PASS that was not read from the server. Exit code 0 when no FAIL, 1 when any FAIL, 2 when the server cannot be
reached or refuses the token. See docs/api-reference.md for the routes, and the testing-guide skill for when to run this.
"""
import argparse, json, os, sys, urllib.error, urllib.request
from pathlib import Path

PASS, WARN, FAIL, SKIP = "PASS", "WARN", "FAIL", "SKIP"
TIMEOUT_S = 20
REPO_VERSION_FILE = Path(__file__).resolve().parent.parent / "VERSION"


class Unreachable(Exception):
    pass


def get(base, path, token):
    """One GET. Returns (status, parsed JSON or text). Raises Unreachable when the server cannot be spoken to at all."""
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(base.rstrip("/") + path, method="GET", headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT_S) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(errors="replace")[:200]
    except (urllib.error.URLError, TimeoutError, OSError) as e:
        raise Unreachable(str(e)) from e
    except json.JSONDecodeError:
        return 200, "not JSON"


def result(name, status, detail):
    return {"check": name, "status": status, "detail": detail}


def expected_version(arg):
    if arg:
        return arg
    try:
        return REPO_VERSION_FILE.read_text().strip()
    except OSError:
        return ""


def check_version(base, token, want):
    code, body = get(base, "/admin/api/version", token)
    if code != 200 or not isinstance(body, dict):
        return result("version", FAIL, f"HTTP {code}")
    have = str(body.get("version", ""))
    if want and have.lstrip("v") != want.lstrip("v"):
        return result("version", FAIL, f"server runs {have} (commit {body.get('commit', '?')}), expected {want}")
    return result("version", PASS, f"{have} (commit {body.get('commit', '?')})")


def check_auth(base, token):
    code, body = get(base, "/admin/api/auth/status", "")
    if code != 200 or not isinstance(body, dict):
        return result("sign-in", FAIL, f"HTTP {code}")
    if not body.get("set_up"):
        return result("sign-in", FAIL, "no passphrase is set up: the admin API is open to setup")
    note = "" if body.get("encrypted") else " (traffic is not encrypted)"
    return result("sign-in", WARN if note else PASS, "set up" + note)


def check_stacks(base, token):
    code, body = get(base, "/admin/api/stacks", token)
    if code != 200:
        return [result("stacks", FAIL, f"HTTP {code}")]
    stacks = body if isinstance(body, list) else (body or {}).get("stacks", [])
    out = []
    for s in stacks:
        sid = s.get("id", "?")
        summary = s.get("summary", {})
        for layer in s.get("layers", []):
            for m in layer.get("modules", []):
                name = f"module {sid}/{m.get('id', '?')}"
                if not m.get("enabled"):
                    out.append(result(name, SKIP, "off" + (f" ({m['blocked']})" if m.get("blocked") else "")))
                elif m.get("blocked"):
                    out.append(result(name, FAIL, f"on but blocked: {m['blocked']}"))
                elif m.get("health") == "stopped":
                    out.append(result(name, FAIL, "on but stopped"))
                else:
                    out.append(result(name, PASS, m.get("health") or "on"))
        out.append(result(f"stack {sid}", FAIL if summary.get("stopped") or summary.get("blocked") else PASS,
                          f"{summary.get('on', 0)} of {summary.get('total', 0)} on, {summary.get('stopped', 0)} stopped, "
                          f"{summary.get('blocked', 0)} blocked"))
    return out or [result("stacks", WARN, "no stacks listed")]


# Status routes that answer {enabled, problem, ...}. A route that is off is SKIP; one that is on must have no problem and, when
# it reports `listening`, must listen.
STATUS_ROUTES = [
    ("router log", "/admin/api/netaudit/router-log"),
    ("decoys", "/admin/api/netaudit/decoys"),
    ("presence check", "/admin/api/netaudit/presence"),
    ("nearby", "/admin/api/netaudit/nearby"),
    ("known-bad list", "/admin/api/netaudit/known-bad"),
    ("outside lookup", "/admin/api/netaudit/outside-lookup"),
    ("capture window", "/admin/api/netaudit/capture-window"),
    ("readiness", "/admin/api/netaudit/readiness"),
]


def judge_status(name, code, body):
    if code != 200 or not isinstance(body, dict):
        return result(name, FAIL, f"HTTP {code}")
    if name == "readiness":
        if body.get("ready") is False:
            return result(name, FAIL, "not ready: " + "; ".join(str(r) for r in body.get("requirements", []))[:200])
        return result(name, PASS, "ready" + (f", {len(body.get('notices', []))} notice(s)" if body.get("notices") else ""))
    if "enabled" in body and not body["enabled"]:
        return result(name, SKIP, "off")
    if body.get("problem"):
        return result(name, FAIL, str(body["problem"])[:200])
    if "listening" in body and not body["listening"]:
        return result(name, FAIL, "on but not listening")
    return result(name, PASS, "on" if "enabled" in body else "answers")


def check_status_routes(base, token):
    out = []
    for name, path in STATUS_ROUTES:
        code, body = get(base, path, token)
        out.append(judge_status(name, code, body))
    return out


def finding_keys(body):
    """Keys of the open findings and of the resolved ones in a /netaudit/findings answer."""
    open_keys = {f["key"] for f in body.get("findings", []) if isinstance(f, dict) and f.get("key")}
    done = body.get("resolved", [])
    resolved = {r["key"] for r in done if isinstance(r, dict) and r.get("key")} | {r for r in done if isinstance(r, str)}
    return open_keys, resolved


def check_findings(base, token, ws, automation, save, compare):
    if not ws or not automation:
        return [result("findings", SKIP, "pass --workspace-id and --automation to read the nightly findings")]
    q = f"?workspace_id={ws}&automation={automation}"
    code, body = get(base, "/admin/api/netaudit/findings" + q, token)
    if code != 200 or not isinstance(body, dict):
        return [result("findings", FAIL, f"HTTP {code}")]
    if not body.get("scanned"):
        return [result("findings", WARN, "this automation has not scanned yet")]
    keys, resolved = finding_keys(body)
    out = [result("findings", PASS, f"{len(keys)} open, {len(resolved)} resolved in the last scan, "
                                    f"{body.get('devices_up', '?')} devices up, {len(body.get('unscanned', []) or [])} unscanned")]
    if save:
        Path(save).write_text(json.dumps({"keys": sorted(keys)}, indent=1))
        out.append(result("baseline", PASS, f"saved {len(keys)} finding keys to {save}"))
    if compare:
        try:
            before = set(json.loads(Path(compare).read_text()).get("keys", []))
        except (OSError, ValueError) as e:
            return out + [result("compare", FAIL, f"cannot read {compare}: {e}")]
        gone, new = sorted(before - keys), sorted(keys - before)
        status = PASS if not gone and not new else WARN
        out.append(result("compare", status, f"{len(gone)} gone, {len(new)} new since {compare}"
                          + (f"; gone: {gone[:5]}" if gone else "") + (f"; new: {new[:5]}" if new else "")))
    return out


def run(args):
    token = args.token or os.environ.get("LLM_PROXY_TOKEN", "")
    results = [check_version(args.base, token, expected_version(args.expect_version)), check_auth(args.base, token)]
    results += check_stacks(args.base, token)
    results += check_status_routes(args.base, token)
    results += check_findings(args.base, token, args.workspace_id, args.automation, args.save_baseline, args.compare)
    return results


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--base", required=True, help="server address, e.g. http://vertex.local:4001")
    p.add_argument("--token", help="admin token (default: $LLM_PROXY_TOKEN)")
    p.add_argument("--expect-version", help="version the server must run (default: the repository's VERSION file)")
    p.add_argument("--workspace-id")
    p.add_argument("--automation", help="the audit automation whose latest findings are read")
    p.add_argument("--save-baseline", help="write today's open finding keys to this file")
    p.add_argument("--compare", help="compare today's open finding keys with a file from --save-baseline")
    p.add_argument("--json", action="store_true", help="print JSON instead of a table")
    args = p.parse_args(argv)
    try:
        results = run(args)
    except Unreachable as e:
        print(f"cannot reach {args.base}: {e}", file=sys.stderr)
        return 2
    if any(r["check"] == "version" and r["detail"] == "HTTP 401" for r in results):
        print("the server refused the token (HTTP 401): set LLM_PROXY_TOKEN to an admin token", file=sys.stderr)
        return 2
    if args.json:
        print(json.dumps(results, indent=1))
    else:
        for r in results:
            print(f"{r['status']:<5} {r['check']:<28} {r['detail']}")
        counts = {s: sum(1 for r in results if r["status"] == s) for s in (PASS, WARN, FAIL, SKIP)}
        print("\n" + ", ".join(f"{n} {s}" for s, n in counts.items()))
    return 1 if any(r["status"] == FAIL for r in results) else 0


if __name__ == "__main__":
    sys.exit(main())
