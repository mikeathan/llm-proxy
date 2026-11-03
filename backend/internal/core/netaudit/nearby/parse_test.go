package nearby

import (
	"encoding/binary"
	"strings"
	"testing"
)

// msg builds a response by hand. These tests show the parser follows the documented layout; they are not captures (the
// ledger's rule: a message a person writes repeats the person's belief), so real fixtures replace them after the probes.
type msg struct {
	b  []byte
	an int
}

func newMsg() *msg { return &msg{b: make([]byte, 12)} }

func name(n string) []byte {
	var out []byte
	for _, l := range strings.Split(n, ".") {
		out = append(append(out, byte(len(l))), l...)
	}
	return append(out, 0)
}

func (m *msg) rec(owner string, typ uint16, data []byte) *msg {
	m.b = append(m.b, name(owner)...)
	m.b = binary.BigEndian.AppendUint16(m.b, typ)
	m.b = binary.BigEndian.AppendUint16(m.b, 1)
	m.b = binary.BigEndian.AppendUint32(m.b, 120)
	m.b = binary.BigEndian.AppendUint16(m.b, uint16(len(data)))
	m.b = append(m.b, data...)
	m.an++
	return m
}

func (m *msg) bytes() []byte {
	binary.BigEndian.PutUint16(m.b[2:], 0x8400)
	binary.BigEndian.PutUint16(m.b[6:], uint16(m.an))
	return m.b
}

func srv(port uint16, target string) []byte {
	return append(binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, 0), 0), port), name(target)...)
}

func TestParseMDNS_ServiceEnumeration(t *testing.T) {
	raw := newMsg().rec("_services._dns-sd._udp.local", 12, name("_ipp._tcp.local")).rec("_services._dns-sd._udp.local", 12, name("_airplay._tcp.local")).bytes()
	f, err := ParseMDNS(raw)
	if err != nil || len(f.Services) != 2 || f.Services[0] != "_ipp._tcp" || f.Services[1] != "_airplay._tcp" {
		t.Fatalf("got %+v %v", f, err)
	}
}

func TestParseMDNS_InstanceNameAndPort(t *testing.T) {
	raw := newMsg().
		rec("_ipp._tcp.local", 12, name("Office Printer._ipp._tcp.local")).
		rec("Office Printer._ipp._tcp.local", 33, srv(631, "printer.local")).
		rec("Office Printer._ipp._tcp.local", 16, []byte{4, 'a', '=', 'b', 'c'}).bytes()
	f, err := ParseMDNS(raw)
	if err != nil || len(f.Names) != 1 || f.Names[0] != "Office Printer" || len(f.Ports) != 1 || f.Ports[0] != 631 || f.Services[0] != "_ipp._tcp" {
		t.Fatalf("got %+v %v", f, err)
	}
}

func TestParseMDNS_HostileInputIsCleanedOrDropped(t *testing.T) {
	hostile := "<img src=x onerror=alert(1)>‮gpj.exe"
	raw := newMsg().
		rec("_services._dns-sd._udp.local", 12, name("not a service type")).
		rec("_services._dns-sd._udp.local", 12, name("_"+strings.Repeat("a", 40)+"._tcp.local")).
		rec("_http._tcp.local", 12, name(hostile+"._http._tcp.local")).bytes()
	f, err := ParseMDNS(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Services) != 1 || f.Services[0] != "_http._tcp" {
		t.Fatalf("a malformed service type got through: %v", f.Services)
	}
	if len(f.Names) != 1 || strings.ContainsRune(f.Names[0], '‮') {
		t.Fatalf("name not cleaned: %q", f.Names)
	}
}

func TestParseMDNS_BoundsEverything(t *testing.T) {
	m := newMsg()
	for i := range 100 {
		svc := "_s" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "._tcp"
		m.rec("_services._dns-sd._udp.local", 12, name(svc+".local"))
		m.rec(svc+".local", 12, name("n"+string(rune('a'+i%26))+string(rune('a'+i/26))+"."+svc+".local"))
		m.rec("x."+svc+".local", 33, srv(uint16(1000+i), "h.local"))
	}
	f, _ := ParseMDNS(m.bytes())
	if len(f.Services) != maxServices || len(f.Names) != maxNames || len(f.Ports) != maxPorts {
		t.Fatalf("kept %d/%d/%d", len(f.Services), len(f.Names), len(f.Ports))
	}
}

func TestParseMDNS_RefusesAQueryATruncatedMessageAndNoise(t *testing.T) {
	for label, raw := range map[string][]byte{"query": MDNSQuestion, "empty": nil, "noise": []byte("HTTP/1.1 200 OK"), "short": {1, 2, 3}} {
		if _, err := ParseMDNS(raw); err == nil {
			t.Errorf("%s: want an error", label)
		}
	}
	whole := newMsg().rec("_services._dns-sd._udp.local", 12, name("_ipp._tcp.local")).bytes()
	if f, err := ParseMDNS(whole[:len(whole)-3]); err == nil && len(f.Services) > 0 {
		t.Errorf("a truncated record must not be read: %+v", f)
	}
}

func FuzzParseMDNS(f *testing.F) {
	f.Add(newMsg().rec("_services._dns-sd._udp.local", 12, name("_ipp._tcp.local")).bytes())
	f.Add(MDNSQuestion)
	f.Fuzz(func(t *testing.T, raw []byte) {
		facts, _ := ParseMDNS(raw)
		if len(facts.Services) > maxServices || len(facts.Names) > maxNames || len(facts.Ports) > maxPorts {
			t.Fatal("bounds broken")
		}
		for _, s := range facts.Services {
			if !serviceRe.MatchString(s) {
				t.Fatalf("service %q does not match the pattern", s)
			}
		}
	})
}

func TestTheQuestionsAreTheDocumentedBytes(t *testing.T) {
	if len(MDNSQuestion) != 46 || len(SSDPSearch) != 101 {
		t.Fatalf("lengths %d %d, documented 46 and 101", len(MDNSQuestion), len(SSDPSearch))
	}
	if !strings.HasSuffix(string(SSDPSearch), "\r\n\r\n") || !strings.Contains(string(SSDPSearch), "ST: upnp:rootdevice") {
		t.Fatal("SSDP search changed")
	}
}
