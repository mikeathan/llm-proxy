# Prepended to every probe by run.sh. Prints what is IN FORCE inside the process (not what the command asked for), so a pasted result
# proves which variant really ran: systemd merges list-type properties, so an override can silently test the wrong thing.
import errno, os, socket


def _errname(e):
    return errno.errorcode.get(e, str(e))


def _status_fields():
    want = ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb", "NoNewPrivs", "Seccomp", "Uid", "Gid", "Groups")
    out = {}
    for line in open("/proc/self/status"):
        key, _, value = line.partition(":")
        if key in want:
            out[key] = value.strip()
    return out


def _try_family(name, family, kind, proto=0):
    try:
        socket.socket(family, kind, proto).close()
        return "ok"
    except OSError as e:
        return _errname(e.errno)


def report_in_force():
    print("in-force: kernel=%s %s" % (os.uname().release, os.uname().machine))
    status = _status_fields()
    for key in ("Uid", "Gid", "Groups", "NoNewPrivs", "Seccomp", "CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
        print("in-force: %s=%s" % (key, status.get(key, "?")))
    eff = int(status.get("CapEff", "0"), 16)
    print("in-force: CAP_NET_ADMIN(12)=%s CAP_NET_RAW(13)=%s" % ("yes" if eff >> 12 & 1 else "no", "yes" if eff >> 13 & 1 else "no"))
    families = (
        ("AF_UNIX", socket.AF_UNIX, socket.SOCK_STREAM, 0),
        ("AF_INET", socket.AF_INET, socket.SOCK_STREAM, 0),
        ("AF_INET6", socket.AF_INET6, socket.SOCK_STREAM, 0),
        ("AF_NETLINK(route)", getattr(socket, "AF_NETLINK", 16), socket.SOCK_RAW, 0),
        ("AF_PACKET(raw)", getattr(socket, "AF_PACKET", 17), socket.SOCK_RAW, socket.htons(3)),
    )
    for name, fam, kind, proto in families:
        print("in-force: socket(%s)=%s" % (name, _try_family(name, fam, kind, proto)))
    print("---", flush=True)
