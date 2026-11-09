"""Tests for verify-deployment.py against a stub server. Run: python3 -m unittest scripts/test_verify_deployment.py"""
import importlib.util, json, tempfile, threading, unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

spec = importlib.util.spec_from_file_location("verify", Path(__file__).with_name("verify-deployment.py"))
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)

ROUTES = {}
SEEN = []


class Stub(BaseHTTPRequestHandler):
    def do_GET(self):
        SEEN.append(("GET", self.path))
        path = self.path.split("?")[0]
        if ROUTES.get("_deny") and path != "/admin/api/auth/status":
            self.send_response(401); self.end_headers(); return
        body = ROUTES.get(path)
        if body is None:
            self.send_response(404); self.end_headers(); return
        raw = json.dumps(body).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers(); self.wfile.write(raw)

    def do_POST(self): SEEN.append(("POST", self.path)); self.send_response(405); self.end_headers()
    do_PUT = do_DELETE = do_POST

    def log_message(self, *a): pass


def healthy():
    return {
        "_deny": False,
        "/admin/api/version": {"version": "0.66.0", "commit": "abc"},
        "/admin/api/auth/status": {"set_up": True, "encrypted": True},
        "/admin/api/stacks": [{"id": "network", "summary": {"on": 1, "total": 2, "stopped": 0, "blocked": 0}, "layers": [{"modules": [
            {"id": "udp_probes", "enabled": True, "health": "working"}, {"id": "decoys", "enabled": False}]}]}],
        "/admin/api/netaudit/router-log": {"enabled": True, "listening": True},
        "/admin/api/netaudit/decoys": {"enabled": False},
        "/admin/api/netaudit/presence": {"enabled": True},
        "/admin/api/netaudit/nearby": {"enabled": False},
        "/admin/api/netaudit/known-bad": {"enabled": False},
        "/admin/api/netaudit/outside-lookup": {"enabled": False},
        "/admin/api/netaudit/capture-window": {"enabled": False},
        "/admin/api/netaudit/readiness": {"ready": True, "requirements": [], "notices": []},
        "/admin/api/netaudit/findings": {"scanned": True, "findings": [{"key": "a"}, {"key": "b"}], "resolved": [], "devices_up": 3},
    }


class VerifyTest(unittest.TestCase):
    def setUp(self):
        ROUTES.clear(); ROUTES.update(healthy()); SEEN.clear()
        self.srv = HTTPServer(("127.0.0.1", 0), Stub)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.base = f"http://127.0.0.1:{self.srv.server_port}"

    def tearDown(self):
        if not getattr(self, "stopped", False):
            self.srv.shutdown(); self.srv.server_close()

    def run_cli(self, *extra):
        import io, contextlib
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = verify.main(["--base", self.base, "--expect-version", "0.66.0", "--json", *extra])
        return code, (json.loads(out.getvalue()) if out.getvalue() else [])

    def status(self, results, name): return next(r["status"] for r in results if r["check"] == name)

    def test_a_healthy_server_passes_and_only_reads(self):
        code, res = self.run_cli()
        self.assertEqual(code, 0)
        self.assertEqual(self.status(res, "version"), "PASS")
        self.assertEqual(self.status(res, "module network/decoys"), "SKIP")
        self.assertEqual({m for m, _ in SEEN}, {"GET"})

    def test_the_wrong_version_fails(self):
        ROUTES["/admin/api/version"] = {"version": "0.65.0", "commit": "x"}
        code, res = self.run_cli()
        self.assertEqual((code, self.status(res, "version")), (1, "FAIL"))

    def test_a_listener_that_is_on_but_not_listening_fails(self):
        ROUTES["/admin/api/netaudit/router-log"] = {"enabled": True, "listening": False}
        code, res = self.run_cli()
        self.assertEqual((code, self.status(res, "router log")), (1, "FAIL"))

    def test_a_stopped_module_fails(self):
        ROUTES["/admin/api/stacks"][0]["layers"][0]["modules"][0]["health"] = "stopped"
        code, res = self.run_cli()
        self.assertEqual((code, self.status(res, "module network/udp_probes")), (1, "FAIL"))

    def test_a_missing_route_is_a_failure_not_a_pass(self):
        del ROUTES["/admin/api/netaudit/presence"]
        _, res = self.run_cli()
        self.assertEqual(self.status(res, "presence check"), "FAIL")

    def test_baseline_compare_reports_new_and_gone_findings(self):
        f = str(Path(tempfile.mkdtemp()) / "night.json")
        self.run_cli("--workspace-id", "1", "--automation", "x", "--save-baseline", f)
        code, res = self.run_cli("--workspace-id", "1", "--automation", "x", "--compare", f)
        self.assertEqual(self.status(res, "compare"), "PASS")
        ROUTES["/admin/api/netaudit/findings"]["findings"] = [{"key": "b"}, {"key": "c"}]
        _, res = self.run_cli("--workspace-id", "1", "--automation", "x", "--compare", f)
        compare = next(r for r in res if r["check"] == "compare")
        self.assertEqual(compare["status"], "WARN")
        self.assertIn("1 gone, 1 new", compare["detail"])

    def test_a_refused_token_exits_2(self):
        ROUTES["_deny"] = True
        code, _ = self.run_cli()
        self.assertEqual(code, 2)

    def test_an_unreachable_server_exits_2(self):
        self.srv.shutdown(); self.srv.server_close()
        self.stopped = True
        self.assertEqual(verify.main(["--base", self.base]), 2)


if __name__ == "__main__":
    unittest.main()
