package netaudit

import (
	"testing"
	"time"

	"llm-proxy/models"
)

var overviewNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func dev(mac, name string, approved bool, firstSeen, lastSeen time.Time) models.NetworkDevice {
	return models.NetworkDevice{MAC: mac, IP: "192.168.1.5", Name: name, Approved: approved, FirstSeen: firstSeen, LastSeen: lastSeen}
}

func TestOverview_NothingLookedAtYetIsUnknownNotQuiet(t *testing.T) {
	o := OverviewOf(OverviewInput{Now: overviewNow})
	if o.Posture != PostureUnknown {
		t.Errorf("posture = %q: an empty network that was never looked at is not 'all clear'", o.Posture)
	}
}

func TestOverview_AQuietNetworkSaysSo(t *testing.T) {
	inv := models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{dev("aa:bb:cc:00:00:01", "Router", true, overviewNow.AddDate(0, -3, 0), overviewNow)}}
	o := OverviewOf(OverviewInput{Now: overviewNow, Inventory: inv, Baseline: models.ScanBaseline{ScannedAt: overviewNow.Add(-time.Hour)}})
	if o.Posture != PostureQuiet || o.Devices.Known != 1 || o.Devices.Unnamed != 0 || !o.Findings.Scanned {
		t.Errorf("overview = %+v", o)
	}
}

func TestOverview_ANewDeviceASeriousFindingOrAChangedRouterNeedsALook(t *testing.T) {
	approved := dev("aa:bb:cc:00:00:01", "Router", true, overviewNow.AddDate(0, -3, 0), overviewNow)
	stranger := dev("aa:bb:cc:00:00:02", "", false, overviewNow.Add(-time.Hour), overviewNow)
	cases := map[string]OverviewInput{
		"new device": {Inventory: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{approved, stranger}}},
		"serious finding": {Inventory: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{approved}},
			Baseline: models.ScanBaseline{ScannedAt: overviewNow, Findings: []models.ScanFinding{{Severity: SeverityHigh}}}},
		"router changed": {Inventory: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{approved}, Gateway: "192.168.1.1", GatewayMAC: "aa:bb:cc:00:00:01", GatewaySeen: "aa:bb:cc:00:00:99"}},
	}
	for name, in := range cases {
		in.Now = overviewNow
		if o := OverviewOf(in); o.Posture != PostureAttention {
			t.Errorf("%s: posture = %q, want attention", name, o.Posture)
		}
	}
}

func TestOverview_CountsFindingsUnnamedDevicesAndNewKeys(t *testing.T) {
	inv := models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		dev("aa:bb:cc:00:00:01", "Router", true, overviewNow.AddDate(0, -3, 0), overviewNow),
		dev("aa:bb:cc:00:00:02", "", true, overviewNow.AddDate(0, -3, 0), overviewNow),
	}}
	b := models.ScanBaseline{ScannedAt: overviewNow, NewKeys: []string{"a", "b"}, Findings: []models.ScanFinding{{Severity: SeverityCritical}, {Severity: SeverityLow}, {Severity: SeverityMedium}}}
	o := OverviewOf(OverviewInput{Now: overviewNow, Inventory: inv, Baseline: b})
	if o.Devices.Unnamed != 1 || o.Findings.Open != 3 || o.Findings.Serious != 1 || o.Findings.New != 2 {
		t.Errorf("overview = %+v", o)
	}
}

func TestOverview_RecentListsNewDevicesAndAddressChangesNewestFirstAndBounded(t *testing.T) {
	var devices []models.NetworkDevice
	for i := range 30 {
		devices = append(devices, dev("aa:bb:cc:00:00:"+string(rune('a'+i%6))+string(rune('a'+i/6)), "", true, overviewNow.Add(-time.Duration(i+1)*time.Hour), overviewNow))
	}
	inv := models.DeviceInventory{Baselined: true, Devices: devices, Changes: []models.AddressChange{{IP: "192.168.1.9", From: "aa", To: "bb", At: overviewNow.Add(-30 * time.Minute)}}}
	o := OverviewOf(OverviewInput{Now: overviewNow, Inventory: inv})
	if len(o.Recent) != maxRecent {
		t.Fatalf("recent = %d, want %d", len(o.Recent), maxRecent)
	}
	if o.Recent[0].Kind != RecentAddressChanged || o.Recent[1].Kind != RecentNewDevice {
		t.Errorf("order = %v, %v", o.Recent[0].Kind, o.Recent[1].Kind)
	}
	for i := 1; i < len(o.Recent); i++ {
		if o.Recent[i].At.After(o.Recent[i-1].At) {
			t.Fatal("not newest first")
		}
	}
}
