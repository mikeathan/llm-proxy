package nmap

import (
	"strings"
	"testing"
)

const udpXML = `<?xml version="1.0"?>
<nmaprun><host><status state="up"/><address addr="192.168.50.1" addrtype="ipv4"/>
<ports>
<port protocol="udp" portid="53"><state state="open"/><service name="domain" method="table"/></port>
<port protocol="udp" portid="123"><state state="open|filtered"/><service name="ntp" method="table"/></port>
<port protocol="udp" portid="1900"><state state="closed"/><service name="upnp" method="table"/></port>
<port protocol="tcp" portid="22"><state state="open"/><service name="ssh"/></port>
</ports></host></nmaprun>`

const osXML = `<?xml version="1.0"?>
<nmaprun><host><status state="up"/><address addr="192.168.50.1" addrtype="ipv4"/>
<os>
<osmatch name="Linux 4.15 - 5.19" accuracy="96" line="1"><osclass type="general purpose" vendor="Linux" osfamily="Linux" osgen="4.X" accuracy="96"/></osmatch>
<osmatch name="Asus RT-AC66U router (Linux 2.6)" accuracy="91" line="2"/>
</os></host></nmaprun>`

func TestParseProbe_KeepsOpenAndOpenFilteredUDPPortsOnly(t *testing.T) {
	probe, err := ParseProbe([]byte(udpXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.UDPPorts) != 2 {
		t.Fatalf("UDPPorts = %+v, want the open and the open|filtered ones", probe.UDPPorts)
	}
	if p := probe.UDPPorts[0]; p.Port != 53 || p.State != "open" || p.Service != "domain" {
		t.Errorf("first port = %+v", p)
	}
	if p := probe.UDPPorts[1]; p.Port != 123 || p.State != "open|filtered" {
		t.Errorf("second port = %+v", p)
	}
}

func TestParseProbe_ReadsOSGuessesBestFirst(t *testing.T) {
	probe, err := ParseProbe([]byte(osXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.OS) != 2 || probe.OS[0].Name != "Linux 4.15 - 5.19" || probe.OS[0].Accuracy != 96 || probe.OS[1].Accuracy != 91 {
		t.Fatalf("OS = %+v", probe.OS)
	}
}

func TestParseProbe_NoGuessIsNotAnError(t *testing.T) {
	probe, err := ParseProbe([]byte(`<nmaprun><host><status state="up"/><address addr="192.168.50.9" addrtype="ipv4"/></host></nmaprun>`))
	if err != nil || len(probe.OS) != 0 || len(probe.UDPPorts) != 0 {
		t.Fatalf("probe = %+v, err = %v", probe, err)
	}
}

func TestParseProbe_TextFromTheDeviceIsCleanedAndBounded(t *testing.T) {
	name := "Evil\u202e\u200b " + strings.Repeat("x", 500)
	probe, err := ParseProbe([]byte(`<nmaprun><host><status state="up"/><address addr="192.168.50.9" addrtype="ipv4"/>` +
		`<os><osmatch name="` + name + `" accuracy="900"/></os></host></nmaprun>`))
	if err != nil {
		t.Fatal(err)
	}
	got := probe.OS[0]
	if strings.ContainsAny(got.Name, "\u202e\u200b") || len([]rune(got.Name)) > maxFieldLen+1 {
		t.Errorf("name not cleaned or bounded: %q", got.Name)
	}
	if got.Accuracy != 100 {
		t.Errorf("accuracy = %d, want it capped at 100", got.Accuracy)
	}
}

func TestParseProbe_MalformedOutputIsAnError(t *testing.T) {
	if _, err := ParseProbe([]byte("PORT STATE SERVICE\n53/udp open domain")); err == nil {
		t.Fatal("plain text was accepted as nmap XML")
	}
}

func TestParseProbe_CapsPortsAndGuesses(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<nmaprun><host><status state="up"/><address addr="192.168.50.9" addrtype="ipv4"/><ports>`)
	for i := 1; i <= maxProbePorts+5; i++ {
		b.WriteString(`<port protocol="udp" portid="` + strings.Repeat("1", 1) + `"><state state="open"/></port>`)
	}
	b.WriteString(`</ports><os>`)
	for i := 0; i < maxOSGuesses+5; i++ {
		b.WriteString(`<osmatch name="Linux" accuracy="90"/>`)
	}
	b.WriteString(`</os></host></nmaprun>`)
	probe, err := ParseProbe([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.UDPPorts) > maxProbePorts || len(probe.OS) > maxOSGuesses {
		t.Errorf("not capped: %d ports, %d guesses", len(probe.UDPPorts), len(probe.OS))
	}
}
