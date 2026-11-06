# What a jailed child can do under the service's own systemd restrictions. It is fed to python3 on stdin by
# scripts/check-sandbox-under-unit.sh, so no file path has to be visible to the restricted process. Each line is one fact.
import ctypes, os, socket, struct, errno

libc = ctypes.CDLL(None, use_errno=True)
U = ctypes.c_ulong  # every argument as a full-width integer: a bare Python int would be cut to 32 bits and a pointer with it
ARCH = {"x86_64": (0xC000003E, 41, 425), "aarch64": (0xC00000B7, 198, 425)}
arch_id, nr_socket, nr_uring = ARCH[os.uname().machine]
NR_SECCOMP = {"x86_64": 317, "aarch64": 277}[os.uname().machine]
PR_SET_NO_NEW_PRIVS, PR_SET_SECCOMP, MODE_FILTER = 38, 22, 2
RET_ALLOW, RET_KILL, RET_ERRNO = 0x7FFF0000, 0x80000000, 0x00050000

def ins(code, k, jt=0, jf=0): return struct.pack("<HBBI", code, jt, jf, k)
# The same program the service installs (platform/sandbox/seccomp_linux.go): refuse socket(AF_UNIX) and io_uring_setup. Indexes: 0 load
# arch, 1 check it, 2 kill, 3 load nr, 4-5 x32 check, 6 io_uring (jump to 11), 7 socket (else jump to 12), 8 load domain, 9 AF_UNIX
# (else jump to 12), 10 EACCES, 11 EPERM, 12 allow.
PROG = b"".join([
    ins(0x20, 4), ins(0x15, arch_id, 1, 0), ins(0x06, RET_KILL), ins(0x20, 0), ins(0x45, 0x40000000, 0, 1), ins(0x06, RET_KILL),
    ins(0x15, nr_uring, 4, 0), ins(0x15, nr_socket, 0, 4), ins(0x20, 16), ins(0x15, 1, 0, 2),
    ins(0x06, RET_ERRNO | errno.EACCES), ins(0x06, RET_ERRNO | errno.EPERM), ins(0x06, RET_ALLOW),
])
buf = ctypes.create_string_buffer(PROG)
fprog = struct.pack("@HL", len(PROG) // 8, ctypes.addressof(buf))
fprog_buf = ctypes.create_string_buffer(fprog)

def say(name, value): print(f"{name}: {value}", flush=True)
def errname(e): return errno.errorcode.get(e, str(e))

say("kernel", os.uname().release + " " + os.uname().machine)
say("set no_new_privs", "ok" if libc.prctl(U(PR_SET_NO_NEW_PRIVS), U(1), U(0), U(0), U(0)) == 0 else errname(ctypes.get_errno()))
# The old route: install an allow-all filter with the seccomp(2) syscall. Under the service's unit this is refused.
allow_all = ctypes.create_string_buffer(ins(0x06, RET_ALLOW))
allow_fprog = ctypes.create_string_buffer(struct.pack("@HL", 1, ctypes.addressof(allow_all)))
r = libc.syscall(U(NR_SECCOMP), U(1), U(0), U(ctypes.addressof(allow_fprog)))
say("seccomp(2) install (the old route)", "ok" if r == 0 else errname(ctypes.get_errno()))
# The new route: prctl.
r = libc.prctl(U(PR_SET_SECCOMP), U(MODE_FILTER), U(ctypes.addressof(fprog_buf)), U(0), U(0))
say("prctl(PR_SET_SECCOMP) install (the route the service uses)", "ok" if r == 0 else errname(ctypes.get_errno()))
if r != 0:
    raise SystemExit(1)
try:
    socket.socket(socket.AF_UNIX).close(); say("socket(AF_UNIX)", "ALLOWED (the filter did nothing)")
except OSError as e: say("socket(AF_UNIX)", errname(e.errno) + " (expected EACCES)")
try:
    a, b = socket.socketpair(); a.close(); b.close(); say("socketpair", "ok (expected)")
except OSError as e: say("socketpair", errname(e.errno) + " (expected ok)")
try:
    socket.socket(socket.AF_INET).close(); say("socket(AF_INET)", "ok (expected)")
except OSError as e: say("socket(AF_INET)", errname(e.errno) + " (expected ok)")
