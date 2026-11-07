# Run from the ASSISTANT'S OWN TERMINAL (an agent child of the service), while standin_listener.py is running. Paste it as
#   python3 - <<'PROBE' ... PROBE
# because the terminal's file jail may not let it read this repository. It prints only facts about its own process and one connect result.
import errno, os, socket

SOCK = "/run/n0-helper-standin/sock"
print("uid=%d gid=%d" % (os.getuid(), os.getgid()))
for line in open("/proc/self/status"):
    if line.split(":")[0] in ("NoNewPrivs", "Seccomp", "CapEff", "CapBnd", "Groups"):
        print(line.strip())
print("cgroup:", open("/proc/self/cgroup").read().strip().splitlines()[:1])
print("open descriptors (an inherited Unix socket would show here):")
for fd in sorted(os.listdir("/proc/self/fd"), key=int):
    try:
        print("  fd %s -> %s" % (fd, os.readlink("/proc/self/fd/" + fd)))
    except OSError:
        pass
try:
    print("stat of the stand-in socket:", oct(os.stat(SOCK).st_mode), "(visible)")
except OSError as e:
    print("stat of the stand-in socket: refused", errno.errorcode.get(e.errno))
try:
    s = socket.socket(socket.AF_UNIX)
    print("socket(AF_UNIX): ok")
    try:
        s.connect(SOCK)
        print("connect: ok  <- the listener should print this process's uid, gid, pid and cgroup")
    except OSError as e:
        print("connect: refused", errno.errorcode.get(e.errno))
except OSError as e:
    print("socket(AF_UNIX): refused", errno.errorcode.get(e.errno))
