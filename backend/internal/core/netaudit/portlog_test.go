package netaudit

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type memPortStore struct{ rec models.DevicePorts }

func (m *memPortStore) Read() (models.DevicePorts, error) { return m.rec, nil }
func (m *memPortStore) Write(r models.DevicePorts) error  { m.rec = r; return nil }

func openHost(ip string, ports ...int) Host {
	h := Host{Addr: netip.MustParseAddr(ip), Up: true}
	for _, p := range ports {
		h.Ports = append(h.Ports, Port{Number: p, Proto: "tcp", State: "open", Product: "device text that must not be kept"})
	}
	return h
}

func macs(pairs map[string]string) func(string) string {
	return func(ip string) string { return pairs[ip] }
}

func TestPortLog_Record(t *testing.T) {
	at := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)
	ip2mac := macs(map[string]string{"192.168.1.5": macTV, "192.168.1.6": macOther})

	t.Run("a device's first scan is a baseline: nothing opened", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		got, err := l.Record([]Host{openHost("192.168.1.5", 22, 80)}, ip2mac, at)
		if err != nil || got != (PortChange{}) {
			t.Fatalf("err=%v got=%+v", err, got)
		}
		if ports := l.For(macTV); len(ports.Ports) != 2 {
			t.Errorf("the ports are kept: %+v", ports)
		}
	})
	t.Run("an opened and a closed port count once each", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		_, _ = l.Record([]Host{openHost("192.168.1.5", 22, 80)}, ip2mac, at)
		got, _ := l.Record([]Host{openHost("192.168.1.5", 22, 443)}, ip2mac, at.Add(24*time.Hour))
		if got != (PortChange{Opened: 1, Closed: 1}) {
			t.Fatalf("got %+v", got)
		}
		if again, _ := l.Record([]Host{openHost("192.168.1.5", 22, 443)}, ip2mac, at.Add(48*time.Hour)); again != (PortChange{}) {
			t.Errorf("an unchanged scan changes nothing: %+v", again)
		}
	})
	t.Run("a device the scan did not finish or find keeps its record", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		_, _ = l.Record([]Host{openHost("192.168.1.5", 22), openHost("192.168.1.6", 80)}, ip2mac, at)
		unscanned := openHost("192.168.1.5")
		unscanned.Unscanned = true
		down := openHost("192.168.1.6")
		down.Up = false
		got, _ := l.Record([]Host{unscanned, down}, ip2mac, at.Add(time.Hour))
		if got != (PortChange{}) || len(l.For(macTV).Ports) != 1 || len(l.For(macOther).Ports) != 1 {
			t.Errorf("unknown is not closed: %+v", got)
		}
		// A partial run that looked at only one device leaves the other alone.
		if got, _ := l.Record([]Host{openHost("192.168.1.5", 22, 8080)}, ip2mac, at.Add(2*time.Hour)); got != (PortChange{Opened: 1}) || len(l.For(macOther).Ports) != 1 {
			t.Errorf("a partial run closes nothing else: %+v", got)
		}
	})
	t.Run("the record follows the device, not the address", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		_, _ = l.Record([]Host{openHost("192.168.1.5", 22)}, ip2mac, at)
		moved := macs(map[string]string{"192.168.1.9": macTV})
		got, _ := l.Record([]Host{openHost("192.168.1.9", 22)}, moved, at.Add(time.Hour))
		if got != (PortChange{}) {
			t.Errorf("the same device on a new address has opened and closed nothing: %+v", got)
		}
	})
	t.Run("a host with no known hardware address is skipped", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		if got, err := l.Record([]Host{openHost("192.168.1.77", 22)}, ip2mac, at); err != nil || got != (PortChange{}) || len(l.All()) != 0 {
			t.Errorf("err=%v got=%+v all=%+v", err, got, l.All())
		}
	})
	t.Run("only the port number and protocol are kept", func(t *testing.T) {
		l := NewPortLog(&memPortStore{})
		_, _ = l.Record([]Host{openHost("192.168.1.5", 22)}, ip2mac, at)
		if got := l.For(macTV).Ports[0]; got != (models.PortRef{Port: 22, Proto: "tcp"}) {
			t.Errorf("got %+v", got)
		}
	})
}

func TestHistory_RecordPortsAddsUpOverADaysAudits(t *testing.T) {
	store := &memHistory{}
	h := NewHistory(store)
	h.now = func() time.Time { return time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC) }
	_ = h.RecordPorts(PortChange{Opened: 2, Closed: 1})
	_ = h.RecordPorts(PortChange{Opened: 1})
	days, _ := h.Series(1)
	if len(days) != 1 || days[0].PortsOpened != 3 || days[0].PortsClosed != 1 {
		t.Errorf("got %+v", days)
	}
}

func TestDigestMessage_SaysHowManyPortsOpenedAndClosedInCountsOnly(t *testing.T) {
	series := []models.NetworkDay{
		{Date: "2026-10-27", Audited: true, PortsOpened: 2},
		{Date: "2026-10-30", Audited: true, PortsOpened: 1, PortsClosed: 3},
		{Date: "2026-10-01", Audited: true, PortsOpened: 9, PortsClosed: 9}, // older than a week
	}
	got := DigestMessage(models.DeviceInventory{}, series, digestNow)
	if want := "3 ports opened and 3 closed"; !strings.Contains(got, want) {
		t.Errorf("missing %q in %q", want, got)
	}
}

type portSpy struct{ got []PortChange }

func (p *portSpy) RecordPorts(c PortChange) error { p.got = append(p.got, c); return nil }

func TestPortLog_HandsWhatChangedToTheHistory(t *testing.T) {
	spy := &portSpy{}
	l := NewPortLog(&memPortStore{}).WithHistory(spy)
	ip2mac := macs(map[string]string{"192.168.1.5": macTV})
	at := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)
	_, _ = l.Record([]Host{openHost("192.168.1.5", 22)}, ip2mac, at)
	_, _ = l.Record([]Host{openHost("192.168.1.5", 22, 80)}, ip2mac, at.Add(time.Hour))
	if len(spy.got) != 1 || spy.got[0] != (PortChange{Opened: 1}) {
		t.Errorf("only a scan that changed something is handed on: %+v", spy.got)
	}
}

// A saved audit hands its hosts to the port record, resolved to hardware addresses through the inventory it just read; a
// failure is reported and never changes the report.
func TestRunReporter_ASavedAuditFeedsThePortRecord(t *testing.T) {
	log := NewPortLog(&memPortStore{})
	snap := Snapshot{Inventory: inventoryWith(models.NetworkDevice{MAC: macTV, IP: "10.0.0.5", FirstSeen: reportTime, LastSeen: reportTime})}
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	r := reporter(store, reportTime).WithInventory(fakeLook{snap: snap}).WithPorts(log)
	r.Augment(scannedHosts(openHost("10.0.0.5", 22, 80)), "ws", "nightly", "ok")
	if got := log.For(macTV).Ports; len(got) != 2 {
		t.Fatalf("the audit's open ports are kept for the device: %+v", got)
	}
}

func TestRunReporter_AScanThatCouldNotBeSavedLeavesThePortRecordAlone(t *testing.T) {
	log := NewPortLog(&memPortStore{})
	snap := Snapshot{Inventory: inventoryWith(models.NetworkDevice{MAC: macTV, IP: "10.0.0.5", FirstSeen: reportTime, LastSeen: reportTime})}
	store := &memBaselines{data: map[string]models.ScanBaseline{}, writeErr: errors.New("read-only")}
	reporter(store, reportTime).WithInventory(fakeLook{snap: snap}).WithPorts(log).Augment(scannedHosts(openHost("10.0.0.5", 22)), "ws", "nightly", "")
	if len(log.All()) != 0 {
		t.Error("only a saved audit counts")
	}
}
