package hosttraffic

import (
	"encoding/binary"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/models"
)

var (
	me     = netip.MustParseAddr("192.168.50.20")
	router = netip.MustParseAddr("192.168.50.1")
	web    = netip.MustParseAddr("93.184.216.34")
	other  = netip.MustParseAddr("151.101.1.69")
	local  = []netip.Addr{me}
)

// ip4 builds an Ethernet + IPv4 + L4 frame; total is the IP total length the header claims (which may be longer than the bytes kept).
func ip4(src, dst netip.Addr, proto byte, total int, l4 []byte) []byte {
	f := make([]byte, 14+20)
	binary.BigEndian.PutUint16(f[12:], 0x0800)
	f[14] = 0x45
	binary.BigEndian.PutUint16(f[16:], uint16(total))
	f[14+9] = proto
	s, d := src.As4(), dst.As4()
	copy(f[14+12:], s[:])
	copy(f[14+16:], d[:])
	return append(f, l4...)
}

func tcp(sport, dport uint16, payload []byte) []byte {
	h := make([]byte, 20)
	binary.BigEndian.PutUint16(h[0:], sport)
	binary.BigEndian.PutUint16(h[2:], dport)
	h[12] = 5 << 4
	return append(h, payload...)
}

func udp(sport, dport uint16, payload []byte) []byte {
	h := make([]byte, 8)
	binary.BigEndian.PutUint16(h[0:], sport)
	binary.BigEndian.PutUint16(h[2:], dport)
	binary.BigEndian.PutUint16(h[4:], uint16(8+len(payload)))
	return append(h, payload...)
}

func ip6(src, dst netip.Addr, next byte, payloadLen int, l4 []byte) []byte {
	f := make([]byte, 14+40)
	binary.BigEndian.PutUint16(f[12:], 0x86dd)
	f[14] = 0x60
	binary.BigEndian.PutUint16(f[14+4:], uint16(payloadLen))
	f[14+6] = next
	s, d := src.As16(), dst.As16()
	copy(f[14+8:], s[:])
	copy(f[14+24:], d[:])
	return append(f, l4...)
}

// dnsAnswer is a response to a query for name with the given A records (and a CNAME first when cname is set).
func dnsAnswer(name string, cname string, addrs ...netip.Addr) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:], 0x1234)
	binary.BigEndian.PutUint16(msg[2:], 0x8180)
	binary.BigEndian.PutUint16(msg[4:], 1)
	an := len(addrs)
	if cname != "" {
		an++
	}
	binary.BigEndian.PutUint16(msg[6:], uint16(an))
	msg = append(msg, encodeName(name)...)
	msg = append(msg, 0, 1, 0, 1) // type A, class IN
	if cname != "" {
		msg = append(msg, 0xc0, 12, 0, 5, 0, 1, 0, 0, 0, 60)
		target := encodeName(cname)
		msg = append(msg, byte(len(target)>>8), byte(len(target)))
		msg = append(msg, target...)
	}
	for _, a := range addrs {
		msg = append(msg, 0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
		b := a.As4()
		msg = append(msg, b[:]...)
	}
	return msg
}

func dnsQuery(name string) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[4:], 1)
	msg = append(msg, encodeName(name)...)
	return append(msg, 0, 1, 0, 1)
}

func encodeName(name string) []byte {
	var out []byte
	for _, l := range strings.Split(name, ".") {
		out = append(out, byte(len(l)))
		out = append(out, l...)
	}
	return append(out, 0)
}

// capture assembles a capture_host output: each frame is (offset ms, sampling rate, bytes).
type kept struct {
	ms     uint32
	sample uint16
	frame  []byte
}

func capture(stop wire.CaptureStop, elapsedMs uint32, frames ...kept) []byte {
	var out []byte
	for _, k := range frames {
		out = wire.AppendHostRecord(out, k.ms, k.sample, k.frame)
	}
	last := uint16(1)
	for _, k := range frames {
		last = max(last, k.sample)
	}
	return wire.AppendHostTrailer(out, wire.HostTrailer{Stop: stop, Frames: uint32(len(frames)), ElapsedMs: elapsedMs, KernelSeen: uint32(len(frames)), Sample: last})
}

func summarize(t *testing.T, data []byte) models.HostTraffic {
	t.Helper()
	s, err := Summarize(data, local)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBytesAreTheIPLengthByDirectionNotTheBytesKept(t *testing.T) {
	out := ip4(me, web, protoTCP, 1500, tcp(51000, 443, nil)) // kept 54 bytes, claims 1500
	in := ip4(web, me, protoTCP, 1400, tcp(443, 51000, nil))
	s := summarize(t, capture(wire.StopWindow, 3000, kept{100, 1, out}, kept{200, 1, in}))
	if s.BytesOut != 1500 || s.BytesIn != 1400 || s.BytesThrough != 0 {
		t.Fatalf("out=%d in=%d through=%d", s.BytesOut, s.BytesIn, s.BytesThrough)
	}
	if len(s.Flows) != 1 || s.Flows[0].Address != "93.184.216.34" || s.Flows[0].Port != 443 || s.Flows[0].Proto != "tcp" || s.Flows[0].Kind != "https" || s.Flows[0].BytesOut != 1500 || s.Flows[0].BytesIn != 1400 || !s.Flows[0].Internet {
		t.Fatalf("flows = %+v", s.Flows)
	}
	if s.Internet != 2900 || s.LocalNetwork != 0 {
		t.Fatalf("internet=%d local=%d", s.Internet, s.LocalNetwork)
	}
}

func TestASampledFrameCountsAsManyBytesAsFramesItStandsFor(t *testing.T) {
	data := ip4(me, web, protoTCP, 1000, tcp(51000, 443, nil))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 4, data}, kept{20, 8, data}))
	if s.BytesOut != 1000*4+1000*8 || s.Sample != 8 {
		t.Fatalf("out=%d sample=%d; each record counts as the rate it was kept at", s.BytesOut, s.Sample)
	}
}

func TestADNSMessageIsNeverMultipliedBecauseItIsNeverSampled(t *testing.T) {
	q := ip4(me, router, protoUDP, 70, udp(40000, 53, dnsQuery("example.com")))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 16, q}))
	if s.BytesOut != 70 || s.Lookups != 1 {
		t.Fatalf("out=%d lookups=%d; a DNS message stands for itself whatever the rate was", s.BytesOut, s.Lookups)
	}
}

func TestADNSAnswerNamesTheAddressesItGivesByTheNameThatWasAsked(t *testing.T) {
	answer := ip4(router, me, protoUDP, 120, udp(53, 40000, dnsAnswer("Example.COM", "edge.cdn.example.net", web)))
	data := ip4(me, web, protoTCP, 900, tcp(51000, 443, nil))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, answer}, kept{20, 1, data}))
	var found bool
	for _, d := range s.Destinations {
		if d.Address == "93.184.216.34" {
			found = true
			if d.Name != "example.com" {
				t.Errorf("destination name = %q, want the asked name in lower case, not the CNAME target", d.Name)
			}
		}
	}
	if !found || s.Flows[0].Name != "example.com" {
		t.Fatalf("destinations = %+v flows = %+v", s.Destinations, s.Flows)
	}
}

func TestANameThatIsNotAPlainDNSNameIsDroppedAndTheAddressIsShown(t *testing.T) {
	for _, bad := range []string{"bad name.example.com", "ex\x01ample.com", "x.in-addr.arpa", strings.Repeat("a", 64) + ".com"} {
		answer := ip4(router, me, protoUDP, 120, udp(53, 40000, dnsAnswer(bad, "", web)))
		data := ip4(me, web, protoTCP, 900, tcp(51000, 443, nil))
		s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, answer}, kept{20, 1, data}))
		for _, d := range s.Destinations {
			if d.Name != "" {
				t.Errorf("%q: kept the name %q", bad, d.Name)
			}
		}
	}
}

func TestADNSMessageThatLoopsOrLiesIsReadSafely(t *testing.T) {
	loop := dnsAnswer("example.com", "", web)
	loop[12] = 0xc0 // the question name points at itself
	loop[13] = 12
	many := dnsAnswer("example.com", "", web)
	binary.BigEndian.PutUint16(many[6:], 60000) // claims far more answers than there are bytes
	for name, msg := range map[string][]byte{"a pointer loop": loop, "a huge answer count": many, "cut in the header": msg6(), "empty": nil} {
		f := ip4(router, me, protoUDP, 100, udp(53, 40000, msg))
		if _, err := Summarize(capture(wire.StopWindow, 1000, kept{10, 1, f}), local); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func msg6() []byte { return []byte{1, 2, 3, 4, 5, 6} }

func TestDNSOverTCPIsReadPastItsLengthPrefix(t *testing.T) {
	msg := dnsAnswer("example.org", "", other)
	framed := append([]byte{byte(len(msg) >> 8), byte(len(msg))}, msg...)
	f := ip4(router, me, protoTCP, 200, tcp(53, 40001, framed))
	data := ip4(me, other, protoTCP, 500, tcp(51000, 443, nil))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, f}, kept{20, 1, data}))
	if s.Flows[0].Name != "example.org" {
		t.Fatalf("flows = %+v", s.Flows)
	}
}

func TestIPv6IsReadAndTheKindsAreFixedWords(t *testing.T) {
	v6me, v6web := netip.MustParseAddr("2001:db8::20"), netip.MustParseAddr("2606:4700::1")
	f := ip6(v6me, v6web, protoTCP, 1000, tcp(51000, 443, nil))
	quic := ip4(me, web, protoUDP, 1200, udp(51001, 443, nil))
	ntp := ip4(me, router, protoUDP, 76, udp(40000, 123, nil))
	ssh := ip4(me, other, protoTCP, 100, tcp(40002, 22, nil))
	ping := ip4(me, web, protoICMP, 84, []byte{8, 0, 0, 0})
	odd := ip4(me, other, protoTCP, 100, tcp(40003, 9999, nil))
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, f}, kept{2, 1, quic}, kept{3, 1, ntp}, kept{4, 1, ssh}, kept{5, 1, ping}, kept{6, 1, odd}), []netip.Addr{me, v6me})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]uint64{}
	for _, k := range s.Kinds {
		got[k.Kind] = k.Bytes
	}
	want := map[string]uint64{"https": 1040, "quic": 1200, "ntp": 76, "ssh": 100, "icmp": 84, "tcp": 100}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if s.Kinds[0].Kind != "quic" {
		t.Errorf("kinds are biggest first, got %v", s.Kinds)
	}
}

func TestTrafficWithAPrivateAddressIsLocalNotInternet(t *testing.T) {
	f := ip4(me, router, protoUDP, 100, udp(40000, 5000, nil))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, f}))
	if s.LocalNetwork != 100 || s.Internet != 0 || s.Flows[0].Internet {
		t.Fatalf("local=%d internet=%d", s.LocalNetwork, s.Internet)
	}
}

func TestAPacketThatIsNeitherFromNorToThisComputerIsCountedAsPassingThrough(t *testing.T) {
	f := ip4(router, web, protoTCP, 700, tcp(51000, 443, nil))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, f}))
	if s.BytesThrough != 700 || s.BytesIn != 0 || s.BytesOut != 0 {
		t.Fatalf("in=%d out=%d through=%d", s.BytesIn, s.BytesOut, s.BytesThrough)
	}
}

func TestBytesAreCountedBySecondAndTheWindowIsPaddedWithQuietSeconds(t *testing.T) {
	f := ip4(me, web, protoTCP, 100, tcp(51000, 443, nil))
	s := summarize(t, capture(wire.StopWindow, 4500, kept{100, 1, f}, kept{900, 1, f}, kept{2500, 1, f}))
	if !reflect.DeepEqual(s.PerSecond, []uint64{200, 0, 100, 0, 0}) {
		t.Fatalf("per second = %v", s.PerSecond)
	}
}

func TestFramesTooShortOrOfAnUnknownShapeAreCountedNotGuessed(t *testing.T) {
	arp := make([]byte, 42)
	binary.BigEndian.PutUint16(arp[12:], 0x0806)
	cut := ip4(me, web, protoTCP, 100, tcp(1, 2, nil))[:14+10]
	s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, []byte{1, 2, 3}}, kept{2, 1, arp}, kept{3, 1, cut}))
	if s.Unreadable != 3 || s.Frames != 3 || s.BytesIn+s.BytesOut != 0 {
		t.Fatalf("unreadable=%d frames=%d", s.Unreadable, s.Frames)
	}
}

func TestTheFlowTableIsBoundedAndTheRestIsCounted(t *testing.T) {
	var frames []kept
	for i := 0; i < maxFlows+100; i++ {
		a := netip.AddrFrom4([4]byte{93, 184, byte(i >> 8), byte(i)})
		frames = append(frames, kept{uint32(i), 1, ip4(me, a, protoTCP, 100+i%7, tcp(51000, 443, nil))})
	}
	s := summarize(t, capture(wire.StopWindow, 5000, frames...))
	if len(s.Flows) != maxShownFlows {
		t.Fatalf("shows %d flows, want %d", len(s.Flows), maxShownFlows)
	}
	if s.OtherFlows != maxFlows+100-maxShownFlows || s.OtherBytes == 0 {
		t.Fatalf("other flows = %d bytes = %d", s.OtherFlows, s.OtherBytes)
	}
	var total uint64
	for _, f := range s.Flows {
		total += f.BytesOut
	}
	if total+s.OtherBytes != s.BytesOut {
		t.Fatalf("the flows and the rest add to %d, but the window sent %d: a byte went missing", total+s.OtherBytes, s.BytesOut)
	}
	for i := 1; i < len(s.Flows); i++ {
		if s.Flows[i].BytesOut+s.Flows[i].BytesIn > s.Flows[i-1].BytesOut+s.Flows[i-1].BytesIn {
			t.Fatalf("flows are not biggest first")
		}
	}
}

func TestTheStopReasonIsSaidInFixedWordsAndACutRunStillSummarises(t *testing.T) {
	s := summarize(t, capture(wire.StopFrameCap, 1000))
	if s.StoppedBy != "frame_limit" {
		t.Errorf("stopped by %q", s.StoppedBy)
	}
	cut := wire.AppendHostRecord(nil, 1, 1, ip4(me, web, protoTCP, 100, tcp(1, 443, nil)))
	s, err := Summarize(cut, local)
	if err != nil || s.StoppedBy != "cut" || s.BytesOut != 100 {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := Summarize([]byte("not a capture"), local); err == nil {
		t.Fatal("bytes that are not a capture were accepted")
	}
}

func TestNoTextFromTheNetworkIsKeptOutsideTheNamesOnDestinationsAndFlows(t *testing.T) {
	allowed := map[string]bool{
		"HostTraffic.Interface": true, "HostTraffic.StoppedBy": true, // from the request and fixed words
		"TrafficKind.Kind":           true, // a fixed word
		"TrafficDestination.Name":    true, // the one place a name may appear (Constitution I.13)
		"TrafficDestination.Address": true,
		"TrafficFlow.Address":        true, "TrafficFlow.Name": true, "TrafficFlow.Proto": true, "TrafficFlow.Kind": true,
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[models.HostTraffic](), reflect.TypeFor[models.TrafficKind](), reflect.TypeFor[models.TrafficDestination](), reflect.TypeFor[models.TrafficFlow]()} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.String && !allowed[typ.Name()+"."+f.Name] {
				t.Errorf("%s.%s is a string field that is not on the list of places text may go", typ.Name(), f.Name)
			}
		}
	}
}

// The helper keeps an ordinary frame only up to the first four bytes after the IP header (Constitution I.13): the addresses, the ports and the
// IP length are all that is read, so a frame cut exactly there must read as it does whole.
func TestAFrameCutRightAfterThePortsIsReadAsItIsWhole(t *testing.T) {
	cut := func(f []byte, ihl int) []byte { return f[:14+ihl*4+4] }
	out := cut(ip4(me, web, protoTCP, 1500, tcp(51000, 443, make([]byte, 1000))), 5)
	in := cut(ip4(web, me, protoUDP, 1200, udp(443, 51001, make([]byte, 1000))), 5)
	v6me, v6web := netip.MustParseAddr("2001:db8::20"), netip.MustParseAddr("2606:4700::1")
	six := ip6(v6me, v6web, protoTCP, 1000, tcp(51000, 22, make([]byte, 900)))[:14+40+4]
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, out}, kept{2, 1, in}, kept{3, 1, six}), []netip.Addr{me, v6me})
	if err != nil {
		t.Fatal(err)
	}
	if s.Unreadable != 0 || s.BytesOut != 1500+1040 || s.BytesIn != 1200 {
		t.Fatalf("unreadable=%d out=%d in=%d", s.Unreadable, s.BytesOut, s.BytesIn)
	}
	byKind := map[string]uint64{}
	for _, k := range s.Kinds {
		byKind[k.Kind] = k.Bytes
	}
	if byKind["https"] != 1500 || byKind["quic"] != 1200 || byKind["ssh"] != 1040 {
		t.Fatalf("kinds = %v", byKind)
	}
}

func TestAFrameThatEndsInsideThePortsIsCountedUnreadable(t *testing.T) {
	short := ip4(me, web, protoTCP, 100, tcp(1, 443, nil))[:14+20+3]
	s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, short}))
	if s.Unreadable != 1 || s.BytesOut != 0 {
		t.Fatalf("unreadable=%d out=%d", s.Unreadable, s.BytesOut)
	}
}
