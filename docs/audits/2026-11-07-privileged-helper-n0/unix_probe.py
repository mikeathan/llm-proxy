# SECONDARY check of the same-user finding (the primary one is the real agent terminal, see the audit's item 5). Under the service's own
# hardening this re-creates the three things that stand between a child and a local Unix socket, one stage each, each in its own forked
# child so they do not stack: A no confinement, B the Landlock file jail alone, C the seccomp filter alone, D a socket made and connected
# BEFORE the filter (an inherited descriptor) used AFTER it. It prints what each stage could do; it guesses nothing. Needs inforce.py.
# The filter is the same program the service installs (platform/sandbox/seccomp_linux.go; see also scripts/sandbox-probe.py).
import ctypes, errno, os, socket, struct, sys, tempfile, threading, warnings

warnings.simplefilter("ignore")  # fork() with the server thread running is intended: each stage is a short-lived child

libc = ctypes.CDLL(None, use_errno=True)
U = ctypes.c_ulong
ARCH = {"x86_64": (0xC000003E, 41, 425, 317), "aarch64": (0xC00000B7, 198, 425, 277)}[os.uname().machine]
ARCH_ID, NR_SOCKET, NR_URING, NR_SECCOMP = ARCH
NR_LL_CREATE, NR_LL_ADD, NR_LL_RESTRICT = 444, 445, 446
RET_ALLOW, RET_KILL, RET_ERRNO = 0x7FFF0000, 0x80000000, 0x00050000
LL_ALL_FS_V1, LL_READ = 0x1FFF, 0x0D  # every ABI-1 filesystem right handled; execute|read file|read dir granted


def ins(code, k, jt=0, jf=0):
    return struct.pack("<HBBI", code, jt, jf, k)


def install_unix_filter():
    prog = b"".join([ins(0x20, 4), ins(0x15, ARCH_ID, 1, 0), ins(0x06, RET_KILL), ins(0x20, 0), ins(0x45, 0x40000000, 0, 1), ins(0x06, RET_KILL),
                     ins(0x15, NR_URING, 4, 0), ins(0x15, NR_SOCKET, 0, 4), ins(0x20, 16), ins(0x15, 1, 0, 2),
                     ins(0x06, RET_ERRNO | errno.EACCES), ins(0x06, RET_ERRNO | errno.EPERM), ins(0x06, RET_ALLOW)])
    buf = ctypes.create_string_buffer(prog)
    fprog = ctypes.create_string_buffer(struct.pack("@HL", len(prog) // 8, ctypes.addressof(buf)))
    libc.prctl(U(38), U(1), U(0), U(0), U(0))
    return libc.prctl(U(22), U(2), U(ctypes.addressof(fprog)), U(0), U(0)) == 0


def landlock_abi():
    r = libc.syscall(U(NR_LL_CREATE), U(0), U(0), U(1))
    return r if r >= 0 else "unavailable(%s)" % errno.errorcode.get(ctypes.get_errno(), "?")


def install_landlock(read_dirs):
    attr = ctypes.create_string_buffer(struct.pack("=Q", LL_ALL_FS_V1))
    fd = libc.syscall(U(NR_LL_CREATE), U(ctypes.addressof(attr)), U(8), U(0))
    if fd < 0:
        return False
    for d in read_dirs:
        if not os.path.isdir(d):
            continue
        dfd = os.open(d, os.O_PATH)
        rule = ctypes.create_string_buffer(struct.pack("=Qi", LL_READ, dfd), 12)
        libc.syscall(U(NR_LL_ADD), U(fd), U(1), U(ctypes.addressof(rule)), U(0))
        os.close(dfd)
    libc.prctl(U(38), U(1), U(0), U(0), U(0))
    return libc.syscall(U(NR_LL_RESTRICT), U(fd), U(0)) == 0


def err(e):
    return errno.errorcode.get(e.errno, str(e)) if isinstance(e, OSError) else repr(e)


def connect_result(path):
    try:
        c = socket.socket(socket.AF_UNIX)
        c.connect(path)
        c.close()
        return "connect ok"
    except OSError as e:
        return "connect refused: " + err(e)


def read_result(path):
    try:
        open(path).close()
        return "read ok"
    except OSError as e:
        return "read refused: " + err(e)


def stage(name, fn):
    sys.stdout.flush()
    pid = os.fork()
    if pid == 0:
        try:
            print("stage %s: %s" % (name, fn()), flush=True)
        except BaseException as e:  # a stage must report, never crash the probe
            print("stage %s: error %r" % (name, e), flush=True)
        os._exit(0)
    os.waitpid(pid, 0)


def stage_b(path, outside_file):
    ok = install_landlock(["/usr", "/lib", "/lib64"])
    return "landlock installed=%s; %s; %s (both paths are outside the grant)" % (ok, read_result(outside_file), connect_result(path))


def stage_c(path):
    ok = install_unix_filter()
    try:
        socket.socket(socket.AF_UNIX).close()
        made = "socket(AF_UNIX) ok"
    except OSError as e:
        made = "socket(AF_UNIX) refused: " + err(e)
    try:
        a, b = socket.socketpair()
        a.close(); b.close()
        pair = "socketpair ok"
    except OSError as e:
        pair = "socketpair refused: " + err(e)
    return "filter installed=%s; %s; %s" % (ok, made, pair)


def stage_d(path):
    c = socket.socket(socket.AF_UNIX)
    c.connect(path)
    ok = install_unix_filter() and install_landlock(["/usr", "/lib", "/lib64"])
    try:
        c.send(b"x")
        used = "send on the inherited connected descriptor ok"
    except OSError as e:
        used = "send refused: " + err(e)
    return "filter+landlock installed=%s; %s" % (ok, used)


def serve(server, log):
    held = []
    while True:
        try:
            conn, _ = server.accept()
        except OSError:
            return
        pid, uid, gid = struct.unpack("3i", conn.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i")))
        log.append("peer uid=%d gid=%d" % (uid, gid))
        held.append(conn)  # kept open so stage D's send is judged by the filter, not by a closed peer


if __name__ == "__main__":
    report_in_force()
    print("landlock_abi=%s" % landlock_abi())
    workdir = tempfile.mkdtemp(prefix="n0-unix-")
    path = os.path.join(workdir, "sock")
    outside_file = os.path.join(workdir, "outside.txt")
    open(outside_file, "w").write("x")
    server = socket.socket(socket.AF_UNIX)
    server.bind(path)
    server.listen(8)
    log = []
    threading.Thread(target=serve, args=(server, log), daemon=True).start()
    stage("A (no confinement)", lambda: connect_result(path))
    stage("B (Landlock file jail only)", lambda: stage_b(path, outside_file))
    stage("C (seccomp Unix-socket filter only)", lambda: stage_c(path))
    stage("D (descriptor made before the filter)", lambda: stage_d(path))
    print("server saw: %s" % "; ".join(log))
    server.close()
    os.unlink(path); os.unlink(outside_file); os.rmdir(workdir)
