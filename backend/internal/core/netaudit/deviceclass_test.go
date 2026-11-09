package netaudit

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestClassOf_NamesAKindOnlyFromSignalsThatNameOne(t *testing.T) {
	cases := map[string]struct {
		e    ClassEvidence
		want DeviceClass
	}{
		"printer by service":   {ClassEvidence{Services: []string{"_ipp._tcp", "_http._tcp"}}, ClassPrinter},
		"printer by port":      {ClassEvidence{Ports: []int{80, 9100}}, ClassPrinter},
		"printer by maker":     {ClassEvidence{Vendor: "Brother Industries, Ltd."}, ClassPrinter},
		"camera by rtsp":       {ClassEvidence{Services: []string{"_rtsp._tcp"}}, ClassCamera},
		"camera by port":       {ClassEvidence{Ports: []int{554}}, ClassCamera},
		"camera by maker":      {ClassEvidence{Vendor: "Hangzhou Hikvision Digital Technology Co.,Ltd."}, ClassCamera},
		"cast device":          {ClassEvidence{Services: []string{"_googlecast._tcp"}}, ClassMedia},
		"esphome device":       {ClassEvidence{Services: []string{"_esphomelib._tcp"}}, ClassSmartHome},
		"homekit accessory":    {ClassEvidence{Services: []string{"_hap._tcp"}}, ClassSmartHome},
		"smart-home maker":     {ClassEvidence{Vendor: "Espressif Inc."}, ClassSmartHome},
		"two signals one kind": {ClassEvidence{Vendor: "Hikvision", Services: []string{"_rtsp._tcp"}, Ports: []int{554}}, ClassCamera},
	}
	for name, c := range cases {
		if got := ClassOf(c.e); got != c.want {
			t.Errorf("%s: class = %q, want %q", name, got, c.want)
		}
	}
}

func TestClassOf_SaysNothingForAComputerAPhoneOrWhenTheSignalsDisagree(t *testing.T) {
	for name, e := range map[string]ClassEvidence{
		"nothing":                           {},
		"a Mac that casts audio":            {Vendor: "Apple, Inc.", Services: []string{"_airplay._tcp", "_spotify-connect._tcp", "_smb._tcp"}},
		"an ordinary web server":            {Ports: []int{22, 80, 443}},
		"a camera maker and a printer port": {Vendor: "Hikvision", Ports: []int{9100}},
		"a printer that casts":              {Services: []string{"_ipp._tcp", "_googlecast._tcp"}},
	} {
		if got := ClassOf(e); got != "" {
			t.Errorf("%s: class = %q, want none", name, got)
		}
	}
}

func TestClassOf_AnUnknownServiceTypeOrAMakerWithAClassWordInsideItIsNotAClass(t *testing.T) {
	if got := ClassOf(ClassEvidence{Services: []string{"_ipp._tcp.evil", "ipp", "_ipp"}}); got != "" {
		t.Errorf("class = %q from a service that is not exactly a listed type", got)
	}
	if got := ClassOf(ClassEvidence{Services: []string{"_IPP._TCP"}}); got != ClassPrinter {
		t.Errorf("service names are not case sensitive, class = %q", got)
	}
	if got := ClassOf(ClassEvidence{Vendor: "Brotherhood Computing"}); got != "" {
		t.Errorf("class = %q from a maker that only starts with a listed word", got)
	}
}

// hikvisionPrefix stands in for the registry the Network stack owns: it names the camera maker for the one prefix these tests use.
var hikvisionPrefix = VendorLookup(func(mac string) string {
	if strings.HasPrefix(mac, "00:bc:99") {
		return "Hangzhou Hikvision Digital Technology Co.,Ltd."
	}
	return ""
})

func classDevice(mac, ip string) models.NetworkDevice {
	return models.NetworkDevice{MAC: mac, IP: ip, FirstSeen: time.Now(), LastSeen: time.Now()}
}

func classHost(addr string, ports ...int) Host {
	h := Host{Addr: netip.MustParseAddr(addr), Up: true}
	for _, p := range ports {
		h.Ports = append(h.Ports, Port{Number: p, Proto: "tcp", State: "open"})
	}
	return h
}

func TestClassMismatchFindings_ACameraWithRemoteLoginOpenIsOneFindingThatListsEachPort(t *testing.T) {
	devices := []models.NetworkDevice{classDevice("00:bc:99:00:00:01", "192.168.1.30")} // Hikvision's prefix in the bundled registry
	got := ClassMismatchFindings(devices, []Host{classHost("192.168.1.30", 80, 554, 22, 23)}, models.NearbyState{}, hikvisionPrefix)
	if len(got) != 1 {
		t.Fatalf("want one finding, got %+v", got)
	}
	f := got[0]
	if f.Device != "192.168.1.30" || f.Check != CheckClassMismatch || f.Severity != SeverityMedium || f.Port != 0 {
		t.Errorf("finding = %+v", f)
	}
	for _, want := range []string{"camera", "remote login (SSH, port 22)", "Telnet (port 23)"} {
		if !strings.Contains(f.Evidence, want) {
			t.Errorf("evidence lacks %q: %s", want, f.Evidence)
		}
	}
	if _, ok := GuideFor(CheckClassMismatch); !ok || CheckTitle(CheckClassMismatch) == "" {
		t.Error("the check needs a title and a guide")
	}
}

func TestClassMismatchFindings_UsesWhatTheDeviceAnnouncedAndNeverReportsWithoutAnAdminPort(t *testing.T) {
	state := models.NearbyState{Devices: []models.NearbyDevice{{IP: "192.168.1.40", Services: []string{"_esphomelib._tcp"}}}}
	got := ClassMismatchFindings(nil, []Host{classHost("192.168.1.40", 6053, 3389)}, state, hikvisionPrefix)
	if len(got) != 1 || !strings.Contains(got[0].Evidence, "smart-home device") || !strings.Contains(got[0].Evidence, "remote desktop (port 3389)") {
		t.Fatalf("findings = %+v", got)
	}
	if quiet := ClassMismatchFindings(nil, []Host{classHost("192.168.1.40", 6053)}, state, hikvisionPrefix); len(quiet) != 0 {
		t.Errorf("a smart-home device with only its own port open is not a finding: %+v", quiet)
	}
}

func TestClassMismatchFindings_ReadsOnlyOpenTcpPortsOfHostsTheScanFinishedAndKnowsNoKindForAnOrdinaryComputer(t *testing.T) {
	cam := []models.NetworkDevice{classDevice("00:bc:99:00:00:01", "192.168.1.30")}
	unscanned := classHost("192.168.1.30", 554, 22)
	unscanned.Unscanned = true
	if got := ClassMismatchFindings(cam, []Host{unscanned}, models.NearbyState{}, hikvisionPrefix); len(got) != 0 {
		t.Errorf("an unfinished scan says nothing: %+v", got)
	}
	closed := classHost("192.168.1.30", 554)
	closed.Ports = append(closed.Ports, Port{Number: 22, Proto: "tcp", State: "closed"}, Port{Number: 23, Proto: "udp", State: "open"})
	if got := ClassMismatchFindings(cam, []Host{closed}, models.NearbyState{}, hikvisionPrefix); len(got) != 0 {
		t.Errorf("a closed port and a UDP port are not an open remote login: %+v", got)
	}
	if got := ClassMismatchFindings(nil, []Host{classHost("192.168.1.9", 22, 80, 443)}, models.NearbyState{}, hikvisionPrefix); len(got) != 0 {
		t.Errorf("a computer with SSH is not a mismatch: %+v", got)
	}
}
