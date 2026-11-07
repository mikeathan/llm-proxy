# Passive link-layer capture on one interface for a fixed window. HEADERS ONLY: it reads at most 128 bytes of each frame, never parses the
# BOOTP/DHCP payload, never enables promiscuous mode, never sends, and prints only counts (no addresses). Needs inforce.py prepended.
# usage: python3 - <iface> <seconds>        python3 - --selftest
import socket, struct, sys, time

SNAP = 128
ETH_ARP, ETH_IP4, ETH_IP6 = 0x0806, 0x0800, 0x86DD
NDP = {133: "ndp_router_solicit", 134: "ndp_router_advert", 135: "ndp_neighbor_solicit", 136: "ndp_neighbor_advert", 137: "ndp_redirect"}
PKTTYPE = {0: "to_this_host", 1: "broadcast", 2: "multicast", 3: "other_host", 4: "sent_by_this_host"}
IFF_PROMISC = 0x100


def _udp_labels(sport, dport, dst_is_broadcast):
    labels = []
    cast = "broadcast" if dst_is_broadcast else "not_broadcast"
    if (sport, dport) == (67, 68):
        labels += ["dhcp_67_to_68", "dhcp_67_to_68_" + cast]
    elif (sport, dport) == (68, 67):
        labels += ["dhcp_68_to_67", "dhcp_68_to_67_" + cast]
    if 5353 in (sport, dport):
        labels.append("mdns")
    if (sport, dport) in ((546, 547), (547, 546)) or dport in (546, 547):
        labels.append("dhcpv6")
    return labels


def classify(frame):
    """Labels for one frame from its Ethernet, ARP, IP and UDP headers only. Returns (labels, arp_sender_hw or None)."""
    if len(frame) < 14:
        return ["too_short"], None
    ethertype = struct.unpack("!H", frame[12:14])[0]
    dst_is_broadcast = frame[0:6] == b"\xff" * 6
    if ethertype == ETH_ARP and len(frame) >= 42:
        op = struct.unpack("!H", frame[20:22])[0]
        labels = ["arp", {1: "arp_request", 2: "arp_reply"}.get(op, "arp_other")]
        if frame[28:32] == frame[38:42]:
            labels.append("arp_gratuitous")
        return labels, frame[22:28]
    if ethertype == ETH_IP4 and len(frame) >= 34:
        ihl = (frame[14] & 0x0F) * 4
        if struct.unpack("!H", frame[20:22])[0] & 0x1FFF:
            return ["ipv4", "ipv4_not_first_fragment"], None
        if frame[23] == 17 and len(frame) >= 14 + ihl + 8:
            sport, dport = struct.unpack("!HH", frame[14 + ihl:14 + ihl + 4])
            return ["ipv4", "ipv4_udp"] + _udp_labels(sport, dport, dst_is_broadcast), None
        return ["ipv4"], None
    if ethertype == ETH_IP6 and len(frame) >= 54:
        nh = frame[20]
        if nh == 58 and len(frame) >= 55:
            return ["ipv6", NDP.get(frame[54], "icmpv6_other")], None
        if nh == 17 and len(frame) >= 62:
            sport, dport = struct.unpack("!HH", frame[54:58])
            return ["ipv6", "ipv6_udp"] + _udp_labels(sport, dport, dst_is_broadcast), None
        return ["ipv6"], None
    return ["other_ethertype"], None


def iface_flags(iface):
    try:
        return int(open("/sys/class/net/%s/flags" % iface).read().strip(), 16)
    except OSError:
        return None


def show_flags(tag, iface):
    flags = iface_flags(iface)
    print("%s: flags=%s promisc=%s" % (tag, "unreadable" if flags is None else hex(flags),
                                       "unknown" if flags is None else ("yes" if flags & IFF_PROMISC else "no")))


def capture(iface, seconds):
    import os
    print("iface=%s wireless=%s window=%ds snap=%d" % (iface, os.path.isdir("/sys/class/net/%s/wireless" % iface), seconds, SNAP))
    show_flags("promisc-before", iface)
    try:
        s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
        s.bind((iface, 0))
    except OSError as e:
        print("RESULT open/bind AF_PACKET: FAILED %s" % os.strerror(e.errno))
        return
    s.settimeout(1.0)
    totals, kinds, per_second, arp_senders = {}, {}, {}, set()
    start = time.monotonic()
    while time.monotonic() - start < seconds:
        try:
            frame, addr = s.recvfrom(SNAP)
        except socket.timeout:
            continue
        sec = int(time.monotonic() - start)
        per_second[sec] = per_second.get(sec, 0) + 1
        pkt = PKTTYPE.get(addr[2], "pkttype_%d" % addr[2])
        totals[pkt] = totals.get(pkt, 0) + 1
        labels, arp_hw = classify(frame)
        if arp_hw is not None:
            arp_senders.add(arp_hw)
        for label in labels:
            key = (label, pkt)
            kinds[key] = kinds.get(key, 0) + 1
    s.close()
    show_flags("promisc-after", iface)
    n = sum(totals.values())
    print("frames_total=%d mean_per_second=%.2f max_per_second=%d" % (n, n / max(seconds, 1), max(per_second.values(), default=0)))
    for k in sorted(totals):
        print("by_pkttype %s=%d" % (k, totals[k]))
    for (label, pkt) in sorted(kinds):
        print("by_label %s [%s]=%d" % (label, pkt, kinds[(label, pkt)]))
    print("arp_distinct_sender_hardware_addresses=%d (a count; none are printed)" % len(arp_senders))


def selftest():
    eth = lambda dst, et: dst + b"\x02\x00\x00\x00\x00\x01" + struct.pack("!H", et)
    arp = lambda op, spa, tpa: eth(b"\xff" * 6, ETH_ARP) + struct.pack("!HHBBH", 1, ETH_IP4, 6, 4, op) + b"\x02\x00\x00\x00\x00\x09" + spa + b"\0" * 6 + tpa
    ip4udp = lambda dst, sport, dport: eth(dst, ETH_IP4) + bytes([0x45, 0]) + struct.pack("!HHHBBH", 40, 0, 0, 64, 17, 0) + b"\0" * 8 + struct.pack("!HHHH", sport, dport, 8, 0)
    cases = [
        (arp(1, b"\x0a\x00\x00\x01", b"\x0a\x00\x00\x02"), ["arp", "arp_request"]),
        (arp(2, b"\x0a\x00\x00\x01", b"\x0a\x00\x00\x01"), ["arp", "arp_reply", "arp_gratuitous"]),
        (ip4udp(b"\xff" * 6, 68, 67), ["ipv4", "ipv4_udp", "dhcp_68_to_67", "dhcp_68_to_67_broadcast"]),
        (ip4udp(b"\x02\x00\x00\x00\x00\x07", 67, 68), ["ipv4", "ipv4_udp", "dhcp_67_to_68", "dhcp_67_to_68_not_broadcast"]),
        (ip4udp(b"\x01\x00\x5e\x00\x00\xfb", 5353, 5353), ["ipv4", "ipv4_udp", "mdns"]),
        (b"\x00" * 5, ["too_short"]),
        (eth(b"\xff" * 6, 0x88CC) + b"\0" * 30, ["other_ethertype"]),
    ]
    bad = 0
    for frame, want in cases:
        got, _ = classify(frame)
        if got != want:
            bad += 1
            print("SELFTEST FAIL want=%s got=%s" % (want, got))
    print("selftest: %d cases, %d failed" % (len(cases), bad))
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    if sys.argv[1:] == ["--selftest"]:
        selftest()
    report_in_force()
    capture(sys.argv[1], int(sys.argv[2]))
