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

// flowKey is one conversation: the remote end and the protocol.
type flowKey struct {
	addr  netip.Addr
	port  uint16
	proto byte
}

// tally is what one pass over the records adds up. The tables are bounded: flows by maxFlows, names by maxNames.
type tally struct {
	local     map[netip.Addr]bool
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
// computer; see Owners). local are this computer's own addresses, which say which way a packet went. The
// interface and the time are the caller's to fill in (Stamp): they come from the request and the host, not from the frames.
func Summarize(data []byte, local []netip.Addr, owners Owners) (models.HostTraffic, error) {
	records, trailer, err := wire.DecodeHostCapture(data)
	if err != nil {
		return models.HostTraffic{}, fmt.Errorf("the capture output could not be read: %w", err)
	}
	t := &tally{
		local: map[netip.Addr]bool{}, flows: map[flowKey]*models.TrafficFlow{}, names: map[netip.Addr]string{}, hellos: map[flowKey]string{}, kinds: map[string]uint64{},
		seconds: make([]uint64, min(int(trailer.ElapsedMs)/msPerSecond+1, maxSeconds)),
	}
	for _, a := range local {
		t.local[a.Unmap()] = true
	}
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
	remote, rport, dir := t.direction(f)
	weight := uint64(r.Sample)
	switch {
	case f.isDNS(): // a DNS message is never sampled away, so it stands for itself
		weight = 1
		t.readNames(f)
	case f.isClientHello(): // nor is the first segment of a ClientHello
		weight = 1
		t.readServerName(f, remote, rport, dir)
	}
	bytes := f.length * weight
	switch dir {
	case dirOut:
		t.out.BytesOut += bytes
	case dirIn:
		t.out.BytesIn += bytes
	default:
		t.out.BytesThrough += bytes
	}
	if isInternet(remote) {
		t.out.Internet += bytes
	} else {
		t.out.LocalNetwork += bytes
	}
	kind := kindOf(f.proto, rport)
	t.kinds[kind] += bytes
	if sec := int(r.OffsetMs) / msPerSecond; sec < len(t.seconds) {
		t.seconds[sec] += bytes
	}
	t.addFlow(flowKey{remote, rport, f.proto}, kind, dir, bytes)
}

type direction int

const (
	dirOut direction = iota
	dirIn
	dirThrough
)

// direction says which end is the remote one and which way the packet went, by this computer's own addresses.
func (t *tally) direction(f frame) (remote netip.Addr, rport uint16, dir direction) {
	switch {
	case t.local[f.src.Unmap()]:
		return f.dst, f.dport, dirOut
	case t.local[f.dst.Unmap()]:
		return f.src, f.sport, dirIn
	}
	return f.dst, f.dport, dirThrough
}

// isInternet says whether the other end is beyond this network: not private, loopback, link-local or multicast.
func isInternet(a netip.Addr) bool {
	a = a.Unmap()
	return !(a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified())
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
func (t *tally) readServerName(f frame, remote netip.Addr, rport uint16, dir direction) {
	if dir != dirOut {
		return
	}
	name, ok := serverName(f.payload)
	if !ok {
		return
	}
	key := flowKey{remote, rport, f.proto}
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
		flow = &models.TrafficFlow{Address: key.addr.Unmap().String(), Port: int(key.port), Proto: protoName(key.proto), Kind: kind, Internet: isInternet(key.addr)}
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
		d = &models.TrafficDestination{Name: f.Name, NameFrom: f.NameFrom, Owner: f.Owner, Address: f.Address, Internet: f.Internet}
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
