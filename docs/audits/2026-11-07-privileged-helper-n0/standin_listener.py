#!/usr/bin/env python3
# THE ONE NON-READ-ONLY STEP of N0 (audit item 5): run as root, it creates a stand-in for the future helper's socket and removes it on exit.
#   path  /run/n0-helper-standin/sock   directory root:llm-proxy 0750, socket root:llm-proxy 0660 (the shape socket activation gives)
# It never reads what a client sends. On each connection it records who connected (SO_PEERCRED uid, gid, pid and that process's cgroup,
# NoNewPrivs and Seccomp lines) and closes. It exits after --max connections or --seconds, and always removes its directory.
# usage: sudo python3 standin_listener.py [--seconds 600] [--max 6] [--group llm-proxy]
import argparse, grp, os, shutil, socket, struct, time

DIR = "/run/n0-helper-standin"


def proc_lines(pid, name, wanted):
    try:
        return [l.strip() for l in open("/proc/%d/%s" % (pid, name)) if l.split(":")[0] in wanted or wanted == ()]
    except OSError as e:
        return ["unreadable: %s" % e.strerror]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--seconds", type=int, default=600)
    ap.add_argument("--max", type=int, default=6)
    ap.add_argument("--group", default="llm-proxy")
    args = ap.parse_args()
    gid = grp.getgrnam(args.group).gr_gid
    os.makedirs(DIR, mode=0o750)
    try:
        os.chown(DIR, 0, gid)
        os.chmod(DIR, 0o750)
        path = DIR + "/sock"
        srv = socket.socket(socket.AF_UNIX)
        srv.bind(path)
        os.chown(path, 0, gid)
        os.chmod(path, 0o660)
        srv.listen(4)
        srv.settimeout(1.0)
        print("listening on %s (root:%s 0660); Ctrl-C or wait %ds to stop and clean up" % (path, args.group, args.seconds), flush=True)
        seen, deadline = 0, time.monotonic() + args.seconds
        while seen < args.max and time.monotonic() < deadline:
            try:
                conn, _ = srv.accept()
            except socket.timeout:
                continue
            pid, uid, g = struct.unpack("3i", conn.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i")))
            conn.close()
            seen += 1
            cg = proc_lines(pid, "cgroup", ())
            st = proc_lines(pid, "status", ("NoNewPrivs", "Seccomp", "Uid", "Gid"))
            print("connection %d: peer uid=%d gid=%d pid=%d | cgroup=%s | %s" % (seen, uid, g, pid, cg[:1], "; ".join(st)), flush=True)
    finally:
        shutil.rmtree(DIR, ignore_errors=True)
        print("removed %s" % DIR, flush=True)


if __name__ == "__main__":
    main()
