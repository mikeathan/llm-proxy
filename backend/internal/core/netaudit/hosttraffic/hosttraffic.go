// Package hosttraffic reads the first bytes of the frames a capture_host run kept into totals, ranked destinations and a flow list
// (Constitution I.13: untrusted data is parsed in the unprivileged service, never in the privileged helper). It reads fixed offsets, counts a
// frame that is too short or of an unknown shape as unreadable, reads a DNS message only to name the addresses an answer gave, and keeps no
// frame: what leaves this package is counts, sums and at most a few hundred vetted names, held in a table that cannot grow past its cap.
package hosttraffic

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/models"
)

const (
	// maxFlows bounds the table of conversations; the biggest are shown and the rest are counted in one bucket.
	maxFlows = 512
	// maxShownFlows and maxShownDestinations are how many of the biggest the summary lists.
	maxShownFlows        = 24
	maxShownDestinations = 10
	msPerSecond          = 1000
	// maxSeconds bounds the per-second series, whatever the trailer claims.
	maxSeconds = wire.HostMaxSeconds + 1
)

// stopWords are the reasons a window ended, in fixed words.
var stopWords = map[wire.CaptureStop]string{
	wire.StopWindow: "window", wire.StopFrameCap: "frame_limit", wire.StopNoInterface: "interface_missing", wire.StopInterfaceDown: "interface_down",
	wire.StopSocketError: "socket_error", wire.StopUnsupported: "unsupported", wire.StopCut: "cut",
}

// flowKey is one conversation: the remote end, the service's port, whether that port is this computer's (served), and the protocol.
type flowKey struct {
	addr   netip.Addr
	port   uint16
	proto  byte
	served bool
	shared bool
}

// ephemeralFloor is the lowest port an operating system picks for the client end of a connection (Linux 32768; macOS and Windows start higher).
// A port below it is taken to be a service's.
const ephemeralFloor = 32768

// Self is what says which packets are this computer's. Addrs are its IP addresses (often only the one the default route sends from, because
// listing interfaces needs netlink, pitfall 65); MACs are its network cards' hardware addresses, read without netlink, which tell the direction
// of IPv6 and of every address the list missed. Port is this program's own listening port: a conversation served on it is the operator's
// browser viewing this page, and is marked so the screen can set it apart.
type Self struct {
	Addrs []netip.Addr
	MACs  [][6]byte
	Port  uint16
}

// tally is what one pass over the records adds up. The tables are bounded: flows by maxFlows, names by maxNames.
type tally struct {
	local     map[netip.Addr]bool
	macs      map[[6]byte]bool
	selfPort  uint16
	out       models.HostTraffic
	flows     map[flowKey]*models.TrafficFlow
	overflow  uint64
	overflowN int
	names     map[netip.Addr]string
	// hellos name a conversation by the server name in the ClientHello this computer sent it (bounded by maxNames).
	hellos  map[flowKey]string
	kinds   map[string]uint64
	seconds []uint64
}

// Summarize decodes one run's output and reads each frame. owners, when not nil, says who holds an internet address (a table kept on this
// computer; see Owners). self says which packets are this computer's (see Self). The
// interface and the time are the caller's to fill in (Stamp): they come from the request and the host, not from the frames.
func Summarize(data []byte, self Self, owners Owners) (models.HostTraffic, error) {
	records, trailer, err := wire.DecodeHostCapture(data)
	if err != nil {
		return models.HostTraffic{}, fmt.Errorf("the capture output could not be read: %w", err)
	}
	t := &tally{
		local: map[netip.Addr]bool{}, macs: map[[6]byte]bool{}, selfPort: self.Port, flows: map[flowKey]*models.TrafficFlow{}, names: map[netip.Addr]string{}, hellos: map[flowKey]string{}, kinds: map[string]uint64{},
		seconds: make([]uint64, min(int(trailer.ElapsedMs)/msPerSecond+1, maxSeconds)),
	}
	for _, a := range self.Addrs {
		t.local[a.Unmap()] = true
	}
	for _, m := range self.MACs {
		if isCardMAC(m) {
			t.macs[m] = true
		}
	}
	t.learnCards(records)
	t.out = models.HostTraffic{
		StoppedBy: stopWords[trailer.Stop], ElapsedMs: int(trailer.ElapsedMs), Frames: len(records),
		KernelSeen: int(trailer.KernelSeen), KernelDropped: int(trailer.KernelDropped), Sample: 1,
	}
	for _, r := range records {
		t.add(r)
	}
	return t.finish(owners), nil
}

// add reads one record into the totals.
func (t *tally) add(r wire.HostRecord) {
	f, ok := parseFrame(r.Frame)
	if !ok {
		t.out.Unreadable++
		return
	}
	t.out.Sample = max(t.out.Sample, int(r.Sample))
	end := t.direction(f)
	remote, dir := end.remote, end.dir
	bytes := f.length * t.weigh(f, end, r.Sample)
	t.countDirection(dir, bytes)
	// A frame to a broadcast or multicast address is shared, whether its IP address says so or only its hardware address does (a subnet's
	// own broadcast address, 192.168.50.255, looks like any other address).
	shared := remote == f.dst && (isGroupMAC(f.dstMAC) || isSharedIP(f.dst))
	if isInternet(remote) && !shared {
		t.out.Internet += bytes
	} else {
		t.out.LocalNetwork += bytes
	}
	key := flowKey{addr: remote, port: end.port, proto: f.proto, served: end.served, shared: shared}
	kind := kindOf(f.proto, end.port)
	t.kinds[kind] += bytes
	if sec := int(r.OffsetMs) / msPerSecond; sec < len(t.seconds) {
		t.seconds[sec] += bytes
	}
	if t.isSelf(key) {
		t.out.SelfBytes += bytes
	}
	t.addFlow(key, kind, dir, bytes)
}

// weigh is how many frames one record stands for, and reads the names it carries: a DNS message and the first segment of a ClientHello are never
// sampled away, so they stand for themselves.
func (t *tally) weigh(f frame, e end, sample uint16) uint64 {
	switch {
	case f.isDNS():
		t.readNames(f)
		return 1
	case f.isClientHello():
		t.readServerName(f, e)
		return 1
	}
	return uint64(sample)
}

// countDirection adds bytes to the total of the way they went.
func (t *tally) countDirection(dir direction, bytes uint64) {
	switch dir {
	case dirOut:
		t.out.BytesOut += bytes
	case dirIn:
		t.out.BytesIn += bytes
	default:
		t.out.BytesThrough += bytes
	}
}

// isSelf says a conversation is this program's own page or API being served.
func (t *tally) isSelf(k flowKey) bool {
	return k.served && k.proto == protoTCP && t.selfPort != 0 && k.port == t.selfPort
}

// learnCards adds the hardware address of every frame that carries one of this computer's IP addresses, so the frames of its other addresses
// (IPv6, a second address the list missed) are told apart by the card. A group or zero address is never taken.
func (t *tally) learnCards(records []wire.HostRecord) {
	if len(t.local) == 0 {
		return
	}
	for _, r := range records {
		f, ok := parseFrame(r.Frame)
		if !ok {
			continue
		}
		if t.local[f.src.Unmap()] && isCardMAC(f.srcMAC) {
			t.macs[f.srcMAC] = true
		}
		if t.local[f.dst.Unmap()] && isCardMAC(f.dstMAC) {
			t.macs[f.dstMAC] = true
		}
	}
}

type direction int

const (
	dirOut direction = iota
	dirIn
	dirThrough
)

// end is the other end of a packet: its address, the conversation's service port, whether that port is this computer's, and which way the
// packet went.
type end struct {
	remote netip.Addr
	port   uint16
	served bool
	dir    direction
}

// direction says which end is the remote one and which way the packet went: by this computer's IP addresses first, then by its network cards. A
// broadcast or multicast frame from another card is one this computer received.
func (t *tally) direction(f frame) end {
	switch {
	case t.local[f.src.Unmap()], t.macs[f.srcMAC]:
		return outgoing(f)
	case t.local[f.dst.Unmap()], t.macs[f.dstMAC]:
		port, served := servicePort(f.dport, f.sport)
		return end{remote: f.src, port: port, served: served, dir: dirIn}
	case len(t.macs) > 0 && isGroupMAC(f.dstMAC):
		// A broadcast or multicast another device sent: received here, and filed under the shared address it went to, so the screen keeps it
		// with the other chatter instead of under the device that sent it.
		return end{remote: f.dst, port: f.dport, dir: dirIn}
	}
	return end{remote: f.dst, port: f.dport, dir: dirThrough}
}

func outgoing(f frame) end {
	port, served := servicePort(f.sport, f.dport)
	return end{remote: f.dst, port: port, served: served, dir: dirOut}
}

// servicePort says which end of a conversation is the service: the end below the ephemeral range when only one is, else the lower port when both
// are below it; served is true when that is this computer's end. Two high ports, or the same port on both ends, leave the remote's port.
func servicePort(local, remote uint16) (port uint16, served bool) {
	switch {
	case local < ephemeralFloor && remote >= ephemeralFloor:
		return local, true
	case local < ephemeralFloor && remote < ephemeralFloor && local < remote:
		return local, true
	}
	return remote, false
}

// isInternet says whether the other end is beyond this network: not private, loopback, link-local, multicast or the limited broadcast.
func isInternet(a netip.Addr) bool {
	a = a.Unmap()
	return !(a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || isSharedIP(a) || a.IsUnspecified())
}

// limitedBroadcast is the IPv4 address every device on the link receives.
var limitedBroadcast = netip.AddrFrom4([4]byte{255, 255, 255, 255})

// isSharedIP says an address is many devices' by its own form: multicast or the limited broadcast. A subnet's broadcast address is only known
// by the frame's hardware address.
func isSharedIP(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsMulticast() || a == limitedBroadcast
}

// readNames reads a DNS message: a question is counted, and the addresses of an answer are named by the name that was asked.
func (t *tally) readNames(f frame) {
	reading := readDNS(f.dnsMessage())
	if reading.question && f.dport == portDNS {
		t.out.Lookups++
	}
	if reading.name == "" {
		return
	}
	for _, a := range reading.addrs {
		if _, known := t.names[a.Unmap()]; known || len(t.names) < maxNames {
			t.names[a.Unmap()] = reading.name
		}
	}
}

// readServerName names the conversation a ClientHello this computer sent opens by the server name in it. A hello that came in names this
// computer's own server, not the other end, so it names nothing. The table is bounded like the DNS names.
func (t *tally) readServerName(f frame, e end) {
	if e.dir != dirOut || e.served {
		return
	}
	name, ok := serverName(f.payload)
	if !ok {
		return
	}
	key := flowKey{addr: e.remote, port: e.port, proto: f.proto}
	if _, known := t.hellos[key]; known || len(t.hellos) < maxNames {
		t.hellos[key] = name
	}
}

// addFlow adds bytes to a conversation. Past the table's cap a new conversation goes to the one bucket for the rest.
func (t *tally) addFlow(key flowKey, kind string, dir direction, bytes uint64) {
	flow, ok := t.flows[key]
	if !ok {
		if len(t.flows) >= maxFlows {
			t.overflow += bytes
			t.overflowN++
			return
		}
		flow = &models.TrafficFlow{
			Address: key.addr.Unmap().String(), Port: int(key.port), Served: key.served, Self: t.isSelf(key), Proto: protoName(key.proto), Kind: kind,
			Internet: isInternet(key.addr) && !key.shared, Shared: key.shared,
		}
		t.flows[key] = flow
	}
	if dir == dirIn {
		flow.BytesIn += bytes
	} else {
		flow.BytesOut += bytes
	}
}

// finish ranks what was added: the biggest flows and destinations, the kinds biggest first, and the rest counted.
func (t *tally) finish(owners Owners) models.HostTraffic {
	out := t.out
	out.DirectionUnknown = out.BytesIn+out.BytesOut == 0 && out.BytesThrough > 0
	flows := make([]models.TrafficFlow, 0, len(t.flows))
	byPlace := map[string]*models.TrafficDestination{}
	held := newHolders(owners)
	for key, f := range t.flows {
		f.Name, f.NameFrom = t.nameOf(key)
		f.Owner = held.of(key.addr, f)
		flows = append(flows, *f)
		addDestination(byPlace, *f)
	}
	out.OwnersUsed, out.Owners, out.Countries = held.used(), held.owners(), held.countries()
	slices.SortFunc(flows, func(a, b models.TrafficFlow) int { return cmp.Compare(flowSize(b), flowSize(a)) })
	out.OtherFlows, out.OtherBytes = t.overflowN, t.overflow
	if len(flows) > maxShownFlows {
		for _, f := range flows[maxShownFlows:] {
			out.OtherFlows++
			out.OtherBytes += flowSize(f)
		}
		flows = flows[:maxShownFlows]
	}
	out.Flows = flows
	out.Destinations = rankDestinations(byPlace)
	out.Kinds = rankKinds(t.kinds)
	out.PerSecond = t.seconds
	return out
}

// nameOf is a conversation's name and where it came from: the server name of its own hello, else the name a DNS answer gave the address.
func (t *tally) nameOf(key flowKey) (name, from string) {
	if name = t.hellos[key]; name != "" {
		return name, models.NameFromTLS
	}
	if name = t.names[key.addr.Unmap()]; name != "" {
		return name, models.NameFromDNS
	}
	return "", ""
}

// flowSize is a flow's bytes both ways.
func flowSize(f models.TrafficFlow) uint64 { return f.BytesIn + f.BytesOut }

// addDestination adds a flow to the destination it belongs to: the name when an answer gave one, else the address.
func addDestination(by map[string]*models.TrafficDestination, f models.TrafficFlow) {
	key := f.Name
	if key == "" {
		key = f.Address
	}
	d, ok := by[key]
	if !ok {
		d = &models.TrafficDestination{Name: f.Name, NameFrom: f.NameFrom, Owner: f.Owner, Address: f.Address, Internet: f.Internet, Shared: f.Shared}
		by[key] = d
	}
	d.Bytes += flowSize(f)
}

func rankDestinations(by map[string]*models.TrafficDestination) []models.TrafficDestination {
	out := make([]models.TrafficDestination, 0, len(by))
	for _, d := range by {
		out = append(out, *d)
	}
	slices.SortFunc(out, func(a, b models.TrafficDestination) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Address, b.Address))
	})
	return out[:min(len(out), maxShownDestinations)]
}

func rankKinds(kinds map[string]uint64) []models.TrafficKind {
	out := make([]models.TrafficKind, 0, len(kinds))
	for k, b := range kinds {
		out = append(out, models.TrafficKind{Kind: k, Bytes: b})
	}
	slices.SortFunc(out, func(a, b models.TrafficKind) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Kind, b.Kind))
	})
	return out
}

// Stamp fills in what the frames cannot say: the time and the interface listened on.
func Stamp(s models.HostTraffic, at time.Time, iface string) models.HostTraffic {
	s.At, s.Interface = at, iface
	return s
}
