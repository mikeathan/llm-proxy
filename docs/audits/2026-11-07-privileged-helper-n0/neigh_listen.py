# Subscribes to the kernel's neighbour-table change notices (RTMGRP_NEIGH) over an AF_NETLINK socket, with no capability, and counts them.
# Read-only: it joins a multicast group and sends nothing. Addresses are printed only with --show-addr (used on the throwaway-namespace
# simulation, where they are made up); on the host it prints counts only. Needs inforce.py prepended.
# usage: python3 - <seconds> [--show-addr]        python3 - --selftest
import socket, struct, sys, time

RTMGRP_NEIGH = 4
RTM_NEWNEIGH, RTM_DELNEIGH = 28, 29
NDA_DST, NDA_LLADDR = 1, 2
NUD = ((0x01, "incomplete"), (0x02, "reachable"), (0x04, "stale"), (0x08, "delay"), (0x10, "probe"), (0x20, "failed"), (0x40, "noarp"), (0x80, "permanent"))
FAMILY = {2: "ipv4", 10: "ipv6", 7: "bridge"}
NLMSG_HDR, NDMSG = struct.Struct("=IHHII"), struct.Struct("=B3xiHBB")


def nud_names(state):
    return "+".join(name for bit, name in NUD if state & bit) or "none"


def parse(buf):
    """Yield one dict per neighbour message in a netlink buffer: kind, family, state, flags, attrs {type: bytes}."""
    off = 0
    while off + NLMSG_HDR.size <= len(buf):
        length, mtype, _flags, _seq, _pid = NLMSG_HDR.unpack_from(buf, off)
        if length < NLMSG_HDR.size:
            return
        if mtype in (RTM_NEWNEIGH, RTM_DELNEIGH):
            fam, ifindex, state, flags, ntype = NDMSG.unpack_from(buf, off + NLMSG_HDR.size)
            attrs, a = {}, off + NLMSG_HDR.size + NDMSG.size
            while a + 4 <= off + length:
                alen, atype = struct.unpack_from("=HH", buf, a)
                if alen < 4:
                    break
                attrs[atype] = buf[a + 4:a + alen]
                a += (alen + 3) & ~3
            yield {"kind": "new" if mtype == RTM_NEWNEIGH else "del", "family": FAMILY.get(fam, str(fam)), "state": nud_names(state), "attrs": attrs}
        off += (length + 3) & ~3


def fmt_addr(raw, family):
    # Written with "-" and "_" so run.sh's address masking leaves these made-up simulation values readable.
    if len(raw) == 4 or (family == "ipv6" and len(raw) == 16):
        return socket.inet_ntop(socket.AF_INET if len(raw) == 4 else socket.AF_INET6, raw).replace(".", "_").replace(":", "-")
    return "-".join("%02x" % b for b in raw)


def listen(seconds, show_addr):
    try:
        s = socket.socket(socket.AF_NETLINK, socket.SOCK_RAW, 0)
        s.bind((0, RTMGRP_NEIGH))
    except OSError as e:
        print("RESULT netlink subscribe: FAILED %s" % e.strerror)
        return
    s.settimeout(1.0)
    counts, overruns, start = {}, 0, time.monotonic()
    while time.monotonic() - start < seconds:
        try:
            buf = s.recv(65535)
        except socket.timeout:
            continue
        except OSError as e:
            overruns += 1
            print("recv error: %s" % e.strerror)
            continue
        for ev in parse(buf):
            key = (ev["kind"], ev["family"], ev["state"], "dst" if NDA_DST in ev["attrs"] else "no_dst", "lladdr" if NDA_LLADDR in ev["attrs"] else "no_lladdr")
            counts[key] = counts.get(key, 0) + 1
            if show_addr:
                print("event %s %s state=%s dst=%s lladdr=%s attrs_present=%s" % (
                    ev["kind"], ev["family"], ev["state"], fmt_addr(ev["attrs"].get(NDA_DST, b""), ev["family"]),
                    fmt_addr(ev["attrs"].get(NDA_LLADDR, b""), ev["family"]), sorted(ev["attrs"])), flush=True)
    print("RESULT subscribed ok for %ds; events=%d recv_errors=%d" % (seconds, sum(counts.values()), overruns))
    for key in sorted(counts):
        print("count %s=%d" % (" ".join(key), counts[key]))


def selftest():
    attr = lambda t, d: struct.pack("=HH", 4 + len(d), t) + d + b"\0" * (-len(d) % 4)
    body = NDMSG.pack(2, 3, 0x04, 0, 1) + attr(NDA_DST, bytes([10, 0, 0, 1])) + attr(NDA_LLADDR, bytes.fromhex("020000000002"))
    msg = NLMSG_HDR.pack(NLMSG_HDR.size + len(body), RTM_NEWNEIGH, 0, 1, 0) + body
    got = list(parse(msg + msg))
    ok = len(got) == 2 and got[0]["state"] == "stale" and fmt_addr(got[0]["attrs"][NDA_LLADDR], "ipv4") == "02-00-00-00-00-02" and fmt_addr(got[0]["attrs"][NDA_DST], "ipv4") == "10_0_0_1"
    print("selftest: %s" % ("ok" if ok else "FAILED %r" % got))
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    if sys.argv[1:] == ["--selftest"]:
        selftest()
    report_in_force()
    listen(int(sys.argv[1]), "--show-addr" in sys.argv[2:])
