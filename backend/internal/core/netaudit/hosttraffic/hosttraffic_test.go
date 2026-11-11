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
	s, err := Summarize(data, Self{Addrs: local}, nil)
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
		if _, err := Summarize(capture(wire.StopWindow, 1000, kept{10, 1, f}), Self{Addrs: local}, nil); err != nil {
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
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, f}, kept{2, 1, quic}, kept{3, 1, ntp}, kept{4, 1, ssh}, kept{5, 1, ping}, kept{6, 1, odd}), Self{Addrs: []netip.Addr{me, v6me}}, nil)
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
	s, err := Summarize(cut, Self{Addrs: local}, nil)
	if err != nil || s.StoppedBy != "cut" || s.BytesOut != 100 {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := Summarize([]byte("not a capture"), Self{Addrs: local}, nil); err == nil {
		t.Fatal("bytes that are not a capture were accepted")
	}
}

func TestNoTextFromTheNetworkIsKeptOutsideTheNamesOnDestinationsAndFlows(t *testing.T) {
	allowed := map[string]bool{
		"HostTraffic.Interface": true, "HostTraffic.StoppedBy": true, // from the request and fixed words
		"TrafficKind.Kind":           true,                                      // a fixed word
		"TrafficDestination.Name":    true,                                      // the one place a name may appear (Constitution I.13)
		"TrafficDestination.Address": true, "TrafficDestination.NameFrom": true, // a fixed word: dns or tls
		"TrafficOwner.Name": true, "TrafficOwner.Country": true, "TrafficCountryShare.Country": true, // the holder's own words (text only), and two capital letters
		"TrafficOwner.Region": true, "TrafficOwner.City": true, // the places table's own words, made plain, shown as text only
		"TrafficOwner.PlaceCountry": true, // two capital letters from the places table
		"TrafficFlow.Address":       true, "TrafficFlow.Name": true, "TrafficFlow.Proto": true, "TrafficFlow.Kind": true, "TrafficFlow.NameFrom": true,
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[models.HostTraffic](), reflect.TypeFor[models.TrafficKind](), reflect.TypeFor[models.TrafficDestination](), reflect.TypeFor[models.TrafficFlow](), reflect.TypeFor[models.TrafficOwner](), reflect.TypeFor[models.TrafficCountryShare]()} {
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
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, out}, kept{2, 1, in}, kept{3, 1, six}), Self{Addrs: []netip.Addr{me, v6me}}, nil)
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

// helloFrame is a frame this computer sent to a server: an IPv4 packet with a TCP segment that starts a ClientHello naming host.
func helloFrame(dst netip.Addr, dport uint16, host string) []byte {
	return ip4(me, dst, protoTCP, 600, tcp(51000, dport, clientHello(sniExt(host))))
}

func TestAServerNameInAClientHelloNamesAnAddressNoDNSAnswerNamed(t *testing.T) {
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, helloFrame(web, 443, "Static.Example.NET")}))
	if len(s.Flows) != 1 || s.Flows[0].Name != "static.example.net" || s.Flows[0].NameFrom != models.NameFromTLS {
		t.Fatalf("flows = %+v, want the name from the hello, said to be from it", s.Flows)
	}
	if len(s.Destinations) != 1 || s.Destinations[0].Name != "static.example.net" || s.Destinations[0].NameFrom != models.NameFromTLS {
		t.Fatalf("destinations = %+v", s.Destinations)
	}
}

func TestTheNameOfTheConnectionBeatsTheNameOfTheAddress(t *testing.T) {
	// One address serves many sites: the DNS answer named the last name asked, the hello names this connection's own.
	answer := ip4(router, me, protoUDP, 120, udp(53, 40000, dnsAnswer("other.example.com", "", web)))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, answer}, kept{20, 1, helloFrame(web, 443, "this.example.com")}))
	if s.Flows[0].Name != "this.example.com" || s.Flows[0].NameFrom != models.NameFromTLS {
		t.Errorf("flow = %+v", s.Flows[0])
	}
	// A flow with no hello of its own keeps the name the answer gave.
	noHello := ip4(me, web, protoTCP, 900, tcp(51001, 8443, nil))
	s = summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, answer}, kept{20, 1, noHello}))
	if s.Flows[0].Name != "other.example.com" || s.Flows[0].NameFrom != models.NameFromDNS {
		t.Errorf("flow = %+v", s.Flows[0])
	}
}

func TestAHelloComingInNamesNothingOnTheOtherEnd(t *testing.T) {
	// A server on this computer receives a hello: its server name is this computer's, not the remote client's.
	in := ip4(other, me, protoTCP, 600, tcp(51000, 443, clientHello(sniExt("mine.example.com"))))
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, in}))
	for _, f := range s.Flows {
		if f.Name != "" {
			t.Errorf("named the remote end %q from a hello it sent to this computer", f.Name)
		}
	}
}

func TestAHelloIsNeverMultipliedBecauseTheFilterNeverSamplesItAway(t *testing.T) {
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 64, helloFrame(web, 443, "a.example.com")}))
	if s.BytesOut != 600 {
		t.Errorf("BytesOut = %d, want the packet's own 600 (a hello stands for itself)", s.BytesOut)
	}
}

func TestAHostileServerNameIsDroppedAndTheNamesAreBounded(t *testing.T) {
	s := summarize(t, capture(wire.StopWindow, 1000, kept{10, 1, helloFrame(web, 443, "bad name\x01.example.com")}))
	if s.Flows[0].Name != "" || s.Flows[0].NameFrom != "" {
		t.Errorf("kept a hostile name: %+v", s.Flows[0])
	}
	var frames []kept
	for i := range maxNames + 40 {
		dst := netip.AddrFrom4([4]byte{93, 184, byte(i >> 8), byte(i)})
		frames = append(frames, kept{uint32(i), 1, helloFrame(dst, 443, "h"+strings.Repeat("a", i%5)+".example.com")})
	}
	s = summarize(t, capture(wire.StopWindow, 1000, frames...))
	named := 0
	for _, f := range s.Flows {
		if f.Name != "" {
			named++
		}
	}
	if named > maxNames {
		t.Errorf("%d flows were named, more than the %d the table allows", named, maxNames)
	}
}

// fakeTable is a stand-in for the downloaded table: two networks, and a record of every address it was asked about.
type fakeTable struct{ asked map[netip.Addr]int }

func (h *fakeTable) owner(a netip.Addr) (models.TrafficOwner, bool) {
	h.asked[a]++
	switch a {
	case web:
		return models.TrafficOwner{ASN: 15133, Name: "EDGECAST", Country: "US"}, true
	case other:
		return models.TrafficOwner{ASN: 54113, Name: "FASTLY", Country: "DE"}, true
	}
	return models.TrafficOwner{}, false
}

func ownedSummary(t *testing.T, frames ...kept) (models.HostTraffic, *fakeTable) {
	t.Helper()
	h := &fakeTable{asked: map[netip.Addr]int{}}
	s, err := Summarize(capture(wire.StopWindow, 1000, frames...), Self{Addrs: local}, h.owner)
	if err != nil {
		t.Fatal(err)
	}
	return s, h
}

func TestAnInternetAddressCarriesItsOwnerAndTheSharesAddUpByHolderAndCountry(t *testing.T) {
	s, _ := ownedSummary(t,
		kept{1, 1, ip4(me, web, protoTCP, 1000, tcp(51000, 443, nil))},
		kept{2, 1, ip4(me, web, protoTCP, 500, tcp(51001, 80, nil))},
		kept{3, 1, ip4(me, other, protoTCP, 300, tcp(51002, 443, nil))},
	)
	if !s.OwnersUsed {
		t.Error("OwnersUsed is false though a table was given")
	}
	for _, f := range s.Flows {
		if f.Owner == nil || f.Owner.ASN == 0 {
			t.Errorf("flow %+v has no owner", f)
		}
	}
	if len(s.Owners) != 2 || s.Owners[0].ASN != 15133 || s.Owners[0].Bytes != 1500 || s.Owners[1].ASN != 54113 || s.Owners[1].Bytes != 300 {
		t.Errorf("owners = %+v, want EDGECAST 1500 then FASTLY 300", s.Owners)
	}
	if len(s.Countries) != 2 || s.Countries[0].Country != "US" || s.Countries[0].Bytes != 1500 || s.Countries[1].Country != "DE" {
		t.Errorf("countries = %+v", s.Countries)
	}
	if s.Destinations[0].Owner == nil || s.Destinations[0].Owner.Name != "EDGECAST" {
		t.Errorf("the biggest destination has no owner: %+v", s.Destinations[0])
	}
}

func TestThisNetworksAddressesAreNeverLookedUpAndNothingIsAskedTwice(t *testing.T) {
	s, h := ownedSummary(t,
		kept{1, 1, ip4(me, router, protoUDP, 100, udp(51000, 53, nil))},
		kept{2, 1, ip4(me, netip.MustParseAddr("224.0.0.251"), protoUDP, 100, udp(5353, 5353, nil))},
		kept{3, 1, ip4(me, web, protoTCP, 100, tcp(51000, 443, nil))},
		kept{4, 1, ip4(me, web, protoTCP, 100, tcp(51001, 80, nil))},
	)
	if h.asked[router] != 0 || h.asked[netip.MustParseAddr("224.0.0.251")] != 0 {
		t.Errorf("a local address was looked up: %v", h.asked)
	}
	if h.asked[web] != 1 {
		t.Errorf("web was looked up %d times; once a window is enough", h.asked[web])
	}
	for _, f := range s.Flows {
		if !f.Internet && f.Owner != nil {
			t.Errorf("a local flow has an owner: %+v", f)
		}
	}
}

func TestAnAddressNotInTheTableHasNoOwnerAndWithoutATableNoOwnerIsSaid(t *testing.T) {
	s, _ := ownedSummary(t, kept{1, 1, ip4(me, netip.MustParseAddr("203.0.113.9"), protoTCP, 100, tcp(51000, 443, nil))})
	if s.Flows[0].Owner != nil || len(s.Owners) != 0 || !s.OwnersUsed {
		t.Errorf("unknown address: %+v owners=%+v used=%v", s.Flows[0], s.Owners, s.OwnersUsed)
	}
	plain := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, ip4(me, web, protoTCP, 100, tcp(51000, 443, nil))}))
	if plain.OwnersUsed || plain.Flows[0].Owner != nil || len(plain.Owners) != 0 {
		t.Errorf("no table, yet owners were said: %+v", plain)
	}
}

func TestTheSharesCoverEveryConversationInTheTableNotOnlyTheBiggestShown(t *testing.T) {
	var frames []kept
	for i := range maxShownFlows + 10 {
		frames = append(frames, kept{uint32(i), 1, ip4(me, web, protoTCP, 100, tcp(uint16(50000+i), 443, nil))})
	}
	s, _ := ownedSummary(t, frames...)
	if len(s.Owners) != 1 || s.Owners[0].Bytes != uint64(100*(maxShownFlows+10)) {
		t.Errorf("owners = %+v, want one holder with every conversation's bytes", s.Owners)
	}
}

var (
	myMAC     = [6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}
	routerMAC = [6]byte{0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	laptop    = netip.MustParseAddr("192.168.50.251")
)

// macs sets a frame's Ethernet destination and source.
func macs(f []byte, src, dst [6]byte) []byte {
	copy(f[0:6], dst[:])
	copy(f[6:12], src[:])
	return f
}

func TestWithoutItsAddressesThisComputerIsKnownByItsNetworkCardsHardwareAddress(t *testing.T) {
	v6me, v6web := netip.MustParseAddr("2001:db8::20"), netip.MustParseAddr("2606:4700::1")
	out := macs(ip6(v6me, v6web, protoTCP, 1000, tcp(51000, 443, nil)), myMAC, routerMAC)
	in := macs(ip6(v6web, v6me, protoTCP, 3000, tcp(443, 51000, nil)), routerMAC, myMAC)
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, out}, kept{2, 1, in}), Self{MACs: [][6]byte{myMAC}}, nil)
	if err != nil || s.BytesOut != 1040 || s.BytesIn != 3040 || s.BytesThrough != 0 || s.DirectionUnknown {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestTheHardwareAddressIsLearnedFromAFrameThatCarriesThisComputersAddress(t *testing.T) {
	v6me, v6web := netip.MustParseAddr("2001:db8::20"), netip.MustParseAddr("2606:4700::1")
	v4 := macs(ip4(me, web, protoTCP, 100, tcp(51000, 443, nil)), myMAC, routerMAC)
	v6 := macs(ip6(v6web, v6me, protoTCP, 960, tcp(443, 51001, nil)), routerMAC, myMAC)
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, v6}, kept{2, 1, v4}), Self{Addrs: local}, nil)
	if err != nil || s.BytesOut != 100 || s.BytesIn != 1000 || s.BytesThrough != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestABroadcastSentByAnotherDeviceIsReceivedAndOneSentHereIsSent(t *testing.T) {
	bcast := [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	theirs := macs(ip4(router, netip.MustParseAddr("192.168.50.255"), protoUDP, 200, udp(7788, 7788, nil)), routerMAC, bcast)
	mine := macs(ip4(netip.MustParseAddr("192.168.50.21"), netip.MustParseAddr("224.0.0.251"), protoUDP, 300, udp(5353, 5353, nil)), myMAC, [6]byte{0x01, 0, 0x5e, 0, 0, 0xfb})
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, theirs}, kept{2, 1, mine}), Self{MACs: [][6]byte{myMAC}}, nil)
	if err != nil || s.BytesIn != 200 || s.BytesOut != 300 || s.BytesThrough != 0 {
		t.Fatalf("%+v %v", s, err)
	}
	for _, f := range s.Flows {
		if f.Address == router.String() {
			t.Fatalf("a broadcast was filed under the device that sent it, not the shared address: %+v", s.Flows)
		}
	}
}

func TestABroadcastOrZeroHardwareAddressIsNeverTakenForThisComputers(t *testing.T) {
	bcast := [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	f := macs(ip4(router, web, protoTCP, 700, tcp(51000, 443, nil)), routerMAC, bcast)
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, f}), Self{MACs: [][6]byte{bcast, {}}}, nil)
	if err != nil || s.BytesThrough != 700 || !s.DirectionUnknown {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestWhenNothingCanBeToldToBeThisComputersTheSummarySaysSo(t *testing.T) {
	f := ip4(laptop, router, protoTCP, 700, tcp(58071, 4001, nil))
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, f}), Self{}, nil)
	if err != nil || !s.DirectionUnknown {
		t.Fatalf("%+v %v", s, err)
	}
	if s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, ip4(me, web, protoTCP, 10, tcp(51000, 443, nil))})); s.DirectionUnknown {
		t.Fatal("a window with this computer's traffic said it could not tell")
	}
}

func TestAConnectionToAServiceHereIsOneConversationOnTheServicesPort(t *testing.T) {
	frames := []kept{
		{1, 1, ip4(laptop, me, protoTCP, 600, tcp(58071, 4001, nil))},
		{2, 1, ip4(me, laptop, protoTCP, 9000, tcp(4001, 58071, nil))},
		{3, 1, ip4(laptop, me, protoTCP, 400, tcp(58072, 4001, nil))},
		{4, 1, ip4(me, laptop, protoTCP, 7000, tcp(4001, 58072, nil))},
		{5, 1, ip4(me, web, protoTCP, 100, tcp(51000, 443, nil))},
	}
	s, err := Summarize(capture(wire.StopWindow, 1000, frames...), Self{Addrs: local, Port: 4001}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var served *models.TrafficFlow
	for i := range s.Flows {
		if s.Flows[i].Address == laptop.String() {
			if served != nil {
				t.Fatalf("the browser's connections were split: %+v", s.Flows)
			}
			served = &s.Flows[i]
		}
	}
	if served == nil || served.Port != 4001 || !served.Served || !served.Self || served.BytesIn != 1000 || served.BytesOut != 16000 {
		t.Fatalf("%+v", served)
	}
	if s.SelfBytes != 17000 {
		t.Fatalf("self bytes %d", s.SelfBytes)
	}
	for _, f := range s.Flows {
		if f.Address == web.String() && (f.Port != 443 || f.Served || f.Self || f.Kind != "https") {
			t.Fatalf("a connection this computer opened lost its service port: %+v", f)
		}
	}
}

func TestTheServicePortIsTheLowOneWhenBothEndsAreBelowTheEphemeralRange(t *testing.T) {
	cases := []struct {
		name               string
		sport, dport, want uint16
		served             bool
	}{
		{"a client here on a low port asks a remote service", 1024, 443, 443, false},
		{"a remote client on a high port asks a service here", 40000, 22, 22, true},
		{"both ends high: the remote port stands", 50000, 60000, 60000, false},
		{"the same port on both ends: the remote one", 5353, 5353, 5353, false},
		{"both ends low, the remote's lower: the remote's is the service", 8080, 600, 600, false},
		{"both ends low, this end's lower: a service here", 1500, 22, 22, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ip4(me, web, protoTCP, 100, tcp(c.sport, c.dport, nil))
			if c.served {
				f = ip4(web, me, protoTCP, 100, tcp(c.sport, c.dport, nil))
			}
			s := summarize(t, capture(wire.StopWindow, 1000, kept{1, 1, f}))
			if got := s.Flows[0]; uint16(got.Port) != c.want || got.Served != c.served {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// The look on vertex (2026-11-11, v0.73.0) put 255.255.255.255 under "on the internet" and the network's own broadcast address 192.168.50.255
// under "on this network". A frame to a broadcast or multicast address, by its IP address or its hardware address, is shared, never internet.
func TestABroadcastIsSharedAndNeverInternetWhetherByItsAddressOrItsHardwareAddress(t *testing.T) {
	bcast := [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	limited := macs(ip4(router, netip.MustParseAddr("255.255.255.255"), protoUDP, 300, udp(9478, 9478, nil)), routerMAC, bcast)
	subnet := macs(ip4(router, netip.MustParseAddr("192.168.50.255"), protoUDP, 200, udp(7788, 7788, nil)), routerMAC, bcast)
	noMAC := ip4(me, netip.MustParseAddr("255.255.255.255"), protoUDP, 100, udp(68, 67, nil))
	unicast := macs(ip4(me, web, protoTCP, 50, tcp(51000, 443, nil)), myMAC, routerMAC)
	s, err := Summarize(capture(wire.StopWindow, 1000, kept{1, 1, limited}, kept{2, 1, subnet}, kept{3, 1, noMAC}, kept{4, 1, unicast}), Self{Addrs: local, MACs: [][6]byte{myMAC}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Internet != 50 {
		t.Fatalf("internet bytes %d, want only the unicast 50", s.Internet)
	}
	for _, f := range s.Flows {
		shared := f.Address != web.String()
		if f.Shared != shared || (f.Internet && shared) {
			t.Errorf("%+v", f)
		}
	}
	for _, d := range s.Destinations {
		if d.Shared != (d.Address != web.String()) {
			t.Errorf("destination %+v", d)
		}
	}
}
