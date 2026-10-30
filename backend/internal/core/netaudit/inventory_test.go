package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

var t0 = time.Date(2026, 10, 30, 2, 0, 0, 0, time.UTC)

func nb(ip, mac string) Neighbor { return Neighbor{Addr: netip.MustParseAddr(ip), MAC: mac} }

func anywhere(netip.Addr) bool { return true }

func table(gateway string, entries ...Neighbor) NeighborTable {
	t := NeighborTable{Entries: entries}
	if gateway != "" {
		t.Gateway = netip.MustParseAddr(gateway)
	}
	return t
}

func find(inv models.DeviceInventory, mac string) *models.NetworkDevice {
	for i := range inv.Devices {
		if inv.Devices[i].MAC == mac {
			return &inv.Devices[i]
		}
	}
	return nil
}

const (
	macRouter = "aa:bb:cc:00:00:01"
	macTV     = "00:11:22:33:44:55"
	macPhone  = "da:a1:19:2c:00:09" // locally administered: a phone's private address
	macOther  = "00:11:22:33:44:66"
)

func TestObserve_FirstLookIsABaseline(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter), nb("192.168.1.20", macTV)), anywhere, t0)
	if !inv.Baselined || len(inv.Devices) != 2 {
		t.Fatalf("inventory = %+v", inv)
	}
	for _, d := range inv.Devices {
		if !d.Approved {
			t.Errorf("a device found by the first look is approved, not new: %+v", d)
		}
	}
	if got := InventoryFindings(inv, t0); len(got) != 0 {
		t.Errorf("a first audit reports a baseline, not %d findings: %+v", len(got), got)
	}
}

func TestObserve_AnEmptyTableIsNotABaseline(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table(""), anywhere, t0)
	if inv.Baselined {
		t.Error("nothing was seen, so the first look has not happened yet")
	}
}

func TestObserve_ANewDeviceIsUnapprovedAndReportedOnce(t *testing.T) {
	base := Observe(models.DeviceInventory{}, table("", nb("192.168.1.1", macRouter)), anywhere, t0)
	later := t0.Add(24 * time.Hour)
	inv := Observe(base, table("", nb("192.168.1.1", macRouter), nb("192.168.1.77", macTV)), anywhere, later)
	d := find(inv, macTV)
	if d == nil || d.Approved || d.FirstSeen != later {
		t.Fatalf("device = %+v", d)
	}
	got := InventoryFindings(inv, later)
	if len(got) != 1 || got[0].Check != CheckNewDevice || got[0].Device != "192.168.1.77" || got[0].Severity != SeverityMedium {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Key() != "192.168.1.77||0|new_device" {
		t.Errorf("key %q", got[0].Key())
	}
	// A device that is looked at again stays the same entry.
	again := Observe(inv, table("", nb("192.168.1.77", macTV)), anywhere, later.Add(time.Minute))
	if len(again.Devices) != 2 || find(again, macTV).FirstSeen != later {
		t.Errorf("seen twice must not be two devices or reset its first-seen: %+v", again.Devices)
	}
}

func TestInventoryFindings_APrivateAddressIsANoteNotAnAlarm(t *testing.T) {
	base := Observe(models.DeviceInventory{}, table("", nb("192.168.1.1", macRouter)), anywhere, t0)
	inv := Observe(base, table("", nb("192.168.1.30", macPhone)), anywhere, t0.Add(time.Hour))
	got := InventoryFindings(inv, t0.Add(time.Hour))
	if len(got) != 1 || got[0].Severity != SeverityLow || !strings.Contains(got[0].Evidence, "private") {
		t.Errorf("a phone's private address is low and says so: %+v", got)
	}
}

func TestInventoryFindings_ANewDeviceThatLeftIsNoLongerReported(t *testing.T) {
	base := Observe(models.DeviceInventory{}, table("", nb("192.168.1.1", macRouter)), anywhere, t0)
	inv := Observe(base, table("", nb("192.168.1.77", macTV)), anywhere, t0.Add(time.Hour))
	if got := InventoryFindings(inv, t0.Add(time.Hour+InventoryActiveWindow+time.Hour)); len(got) != 0 {
		t.Errorf("a visitor gone for a week is not a finding: %+v", got)
	}
}

func TestObserve_OnlyTheScanScopeIsKept(t *testing.T) {
	inScope := func(a netip.Addr) bool { return strings.HasPrefix(a.String(), "192.168.1.") }
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.1", macRouter), nb("172.17.0.2", "02:42:ac:11:00:02")), inScope, t0)
	if len(inv.Devices) != 1 {
		t.Errorf("a docker bridge is not part of the network: %+v", inv.Devices)
	}
}

func TestObserve_AnAddressChangingHandsIsRecordedAndReported(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.20", macTV)), anywhere, t0)
	later := t0.Add(48 * time.Hour)
	inv = Observe(inv, table("", nb("192.168.1.20", macOther)), anywhere, later)
	if len(inv.Changes) != 1 || inv.Changes[0].From != macTV || inv.Changes[0].To != macOther || inv.Changes[0].IP != "192.168.1.20" {
		t.Fatalf("changes = %+v", inv.Changes)
	}
	got := InventoryFindings(inv, later)
	var kinds []string
	for _, f := range got {
		kinds = append(kinds, f.Check)
	}
	if !contains(kinds, CheckMACChanged) || !contains(kinds, CheckNewDevice) {
		t.Errorf("a stranger holding a known device's address is both a new device and a change: %v", kinds)
	}
	// Seen again with the same owner: no second change.
	if again := Observe(inv, table("", nb("192.168.1.20", macOther)), anywhere, later.Add(time.Minute)); len(again.Changes) != 1 {
		t.Errorf("the same owner twice is one change: %+v", again.Changes)
	}
}

func TestInventoryFindings_AChangeBetweenApprovedDevicesIsOrdinaryChurn(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.20", macTV), nb("192.168.1.21", macOther)), anywhere, t0)
	inv = Observe(inv, table("", nb("192.168.1.20", macOther)), anywhere, t0.Add(time.Hour))
	if len(inv.Changes) != 1 {
		t.Fatalf("the change is recorded: %+v", inv.Changes)
	}
	if got := InventoryFindings(inv, t0.Add(time.Hour)); len(got) != 0 {
		t.Errorf("two devices you approved swapping a lease is not a finding: %+v", got)
	}
}

func TestInventoryFindings_AChangeAgesOut(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.20", macTV)), anywhere, t0)
	inv = Observe(inv, table("", nb("192.168.1.20", macOther)), anywhere, t0.Add(time.Hour))
	old := t0.Add(time.Hour + ChangeWindow + time.Hour)
	for _, f := range InventoryFindings(inv, old) {
		if f.Check == CheckMACChanged {
			t.Errorf("an old change is still reported: %+v", f)
		}
	}
	if pruned := Observe(inv, table("", nb("192.168.1.20", macOther)), anywhere, old); len(pruned.Changes) != 0 {
		t.Errorf("old changes are dropped: %+v", pruned.Changes)
	}
}

func TestObserve_TheGatewayChangingMACIsCriticalUntilConfirmed(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter)), anywhere, t0)
	if inv.GatewayMAC != macRouter || inv.Gateway != "192.168.1.1" {
		t.Fatalf("the first look records the gateway: %+v", inv)
	}
	later := t0.Add(time.Hour)
	inv = Observe(inv, table("192.168.1.1", nb("192.168.1.1", macOther)), anywhere, later)
	got := InventoryFindings(inv, later)
	var crit []Finding
	for _, f := range got {
		if f.Check == CheckGatewayChanged {
			crit = append(crit, f)
		}
	}
	if len(crit) != 1 || crit[0].Severity != SeverityCritical || crit[0].Device != "192.168.1.1" {
		t.Fatalf("gateway findings = %+v", crit)
	}
	// It stays until the operator confirms: another look does not make it the new normal.
	inv = Observe(inv, table("192.168.1.1", nb("192.168.1.1", macOther)), anywhere, later.Add(time.Hour))
	if inv.GatewayMAC != macRouter {
		t.Errorf("the approved gateway MAC must not follow the network: %q", inv.GatewayMAC)
	}
	inv = ConfirmGateway(inv)
	for _, f := range InventoryFindings(inv, later.Add(2*time.Hour)) {
		if f.Check == CheckGatewayChanged {
			t.Errorf("a confirmed gateway is not reported: %+v", f)
		}
	}
}

func TestObserve_OneMACOnSeveralAddresses(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter), nb("192.168.1.20", macTV), nb("192.168.1.21", macTV)), anywhere, t0)
	got := InventoryFindings(inv, t0)
	if len(got) != 2 || got[0].Check != CheckDuplicateMAC || got[1].Check != CheckDuplicateMAC {
		t.Fatalf("one finding per address the MAC answers for: %+v", got)
	}
	// The router answering for its own several addresses is not a finding.
	r := Observe(models.DeviceInventory{}, table("192.168.1.1", nb("192.168.1.1", macRouter), nb("192.168.1.2", macRouter)), anywhere, t0)
	if f := InventoryFindings(r, t0); len(f) != 0 {
		t.Errorf("the gateway may have several addresses: %+v", f)
	}
}

func TestInventoryFindings_ANamedDeviceThatStaysAwayIsMissing(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.20", macTV), nb("192.168.1.21", macOther)), anywhere, t0)
	inv.Devices[0].Name = "Living room TV"
	inv.Devices[1].Name = "" // unnamed: nobody is waiting for it
	if got := InventoryFindings(inv, t0.Add(MissingAfter-time.Hour)); len(got) != 0 {
		t.Errorf("not yet: %+v", got)
	}
	got := InventoryFindings(inv, t0.Add(MissingAfter+time.Hour))
	if len(got) != 1 || got[0].Check != CheckDeviceMissing || got[0].Severity != SeverityLow || got[0].Device != "192.168.1.20" {
		t.Fatalf("findings = %+v", got)
	}
	if strings.Contains(got[0].Evidence, "Living room") {
		t.Errorf("a name the operator typed stays out of findings: %q", got[0].Evidence)
	}
}

func TestInventoryFindings_APrivateAddressDeviceIsNeverMissing(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", nb("192.168.1.30", macPhone)), anywhere, t0)
	inv.Devices[0].Name = "Phone"
	if got := InventoryFindings(inv, t0.Add(30*24*time.Hour)); len(got) != 0 {
		t.Errorf("phones come and go: %+v", got)
	}
}

func TestHiddenDeviceFindings(t *testing.T) {
	current := []Neighbor{nb("192.168.1.40", macTV), nb("192.168.1.41", macOther), nb("192.168.1.42", macPhone), nb("192.168.1.43", macRouter)}
	hosts := []Host{
		{Addr: netip.MustParseAddr("192.168.1.40")},           // answers ARP, nothing else
		{Addr: netip.MustParseAddr("192.168.1.41"), Up: true}, // normal
		{Addr: netip.MustParseAddr("192.168.1.42")},           // a phone asleep: private address, ignored
	}
	got := HiddenDeviceFindings(current, hosts)
	if len(got) != 1 || got[0].Check != CheckHiddenDevice || got[0].Device != "192.168.1.40" || got[0].Severity != SeverityLow {
		t.Errorf("findings = %+v (.43 was not swept, so nothing is known about it)", got)
	}
}

type fakeSource struct {
	table NeighborTable
	err   error
}

func (f fakeSource) Neighbors(context.Context) (NeighborTable, error) { return f.table, f.err }

type memInventory struct {
	inv    models.DeviceInventory
	writes int
}

func (m *memInventory) Read() (models.DeviceInventory, error) { return m.inv, nil }
func (m *memInventory) Write(i models.DeviceInventory) error  { m.inv = i; m.writes++; return nil }

func newInventory(src NeighborSource, st *memInventory) *Inventory {
	v := NewInventory(st, src, anywhere)
	v.now = func() time.Time { return t0 }
	return v
}

func TestInventory_RefreshSaysWhenItCouldNotLook(t *testing.T) {
	st := &memInventory{inv: models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{{MAC: macTV, IP: "192.168.1.20", Approved: true}}}}
	v := newInventory(fakeSource{err: errors.New("no table")}, st)
	if _, err := v.Refresh(context.Background()); err == nil {
		t.Fatal("an unreadable table must be an error, not an empty list that reads as clean")
	}
	if st.writes != 0 || len(st.inv.Devices) != 1 {
		t.Errorf("a failed look must leave the inventory alone: %+v (%d writes)", st.inv, st.writes)
	}
}

func TestInventory_NamingApprovesAndForgetRemoves(t *testing.T) {
	st := &memInventory{}
	v := newInventory(fakeSource{table: table("", nb("192.168.1.1", macRouter))}, st)
	if _, err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	v.src = fakeSource{table: table("", nb("192.168.1.1", macRouter), nb("192.168.1.77", macTV))}
	if _, err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if find(st.inv, macTV).Approved {
		t.Fatal("the new device starts unapproved")
	}
	if err := v.Name(macTV, "  Living room TV "); err != nil {
		t.Fatal(err)
	}
	if d := find(st.inv, macTV); d.Name != "Living room TV" || !d.Approved {
		t.Errorf("naming approves: %+v", d)
	}
	if err := v.Name("not-a-mac", "x"); !errors.Is(err, ErrBadMAC) {
		t.Errorf("bad MAC: %v", err)
	}
	if err := v.Name("00:11:22:33:44:99", "x"); !errors.Is(err, ErrNoSuchDevice) {
		t.Errorf("unknown MAC: %v", err)
	}
	if err := v.Name(macTV, strings.Repeat("x", maxDeviceNameLen+1)); !errors.Is(err, ErrBadDeviceName) {
		t.Errorf("long name: %v", err)
	}
	if err := v.Forget(macTV); err != nil || find(st.inv, macTV) != nil {
		t.Errorf("forget: %v %+v", err, st.inv.Devices)
	}
}

func TestInventory_PendingAlertsAreCountedOnceAndOnlyMarkedWhenSent(t *testing.T) {
	st := &memInventory{}
	v := newInventory(fakeSource{table: table("192.168.1.1", nb("192.168.1.1", macRouter))}, st)
	_, _ = v.Refresh(context.Background())
	v.src = fakeSource{table: table("192.168.1.1", nb("192.168.1.1", macOther), nb("192.168.1.77", macTV), nb("192.168.1.78", macPhone))}
	_, _ = v.Refresh(context.Background())

	p, err := v.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if p.NewDevices != 3 || !p.Gateway || p.Empty() {
		t.Fatalf("pending = %+v", p)
	}
	// Not marked yet (the send could have failed): asking again says the same.
	if again, _ := v.Pending(); again.NewDevices != p.NewDevices || again.Gateway != p.Gateway {
		t.Errorf("pending changed without being marked: %+v vs %+v", again, p)
	}
	if err := v.MarkNotified(p); err != nil {
		t.Fatal(err)
	}
	if after, _ := v.Pending(); !after.Empty() {
		t.Errorf("after marking, nothing is pending: %+v", after)
	}
	// A device that appears later is pending again.
	v.src = fakeSource{table: table("192.168.1.1", nb("192.168.1.1", macOther), nb("192.168.1.99", "00:aa:bb:cc:dd:ee"))}
	_, _ = v.Refresh(context.Background())
	if p2, _ := v.Pending(); p2.NewDevices != 1 {
		t.Errorf("one more new device: %+v", p2)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func nb6(ip, mac, iface string) Neighbor {
	return Neighbor{Addr: netip.MustParseAddr(ip), MAC: mac, Interface: iface}
}

func withV6(t NeighborTable, v6 ...Neighbor) NeighborTable { t.Entries6 = v6; return t }

func v4(ip, mac string) Neighbor { n := nb(ip, mac); n.Interface = "wlp2s0"; return n }

func TestObserve_ADualStackDeviceKeepsItsIPv4AddressAndGainsIPv6(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", v4("192.168.1.20", macTV)), anywhere, t0)
	later := t0.Add(time.Hour)
	inv = Observe(inv, withV6(table("", v4("192.168.1.20", macTV)), nb6("fe80::1", macTV, "wlp2s0"), nb6("2001:db8::20", macTV, "wlp2s0")), anywhere, later)
	d := find(inv, macTV)
	if d.IP != "192.168.1.20" || len(d.IPv6) != 2 || len(inv.Shared) != 0 {
		t.Fatalf("device = %+v, shared = %+v", d, inv.Shared)
	}
	if got := InventoryFindings(inv, later); len(got) != 0 {
		t.Errorf("one device with two address families is not a duplicate: %+v", got)
	}
}

func TestObserve_IPv6OnAnotherSegmentIsIgnored(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, withV6(table("", v4("192.168.1.20", macTV)), nb6("fe80::1", macOther, "docker0")), anywhere, t0)
	if find(inv, macOther) != nil {
		t.Errorf("a docker bridge's neighbour is not on this network: %+v", inv.Devices)
	}
}

func TestObserve_ADeviceSeenOnlyOverIPv6IsNewButHasNoFindingKey(t *testing.T) {
	base := Observe(models.DeviceInventory{}, table("", v4("192.168.1.20", macTV)), anywhere, t0)
	later := t0.Add(time.Hour)
	inv := Observe(base, withV6(table("", v4("192.168.1.20", macTV)), nb6("fe80::9", macOther, "wlp2s0")), anywhere, later)
	d := find(inv, macOther)
	if d == nil || d.IP != "" || d.Approved || len(d.IPv6) != 1 {
		t.Fatalf("device = %+v", d)
	}
	if got := InventoryFindings(inv, later); len(got) != 0 {
		t.Errorf("a finding needs an IPv4 address to be keyed by: %+v", got)
	}
	p := pendingOf(inv, later)
	if p.NewDevices != 1 {
		t.Errorf("the operator is still told about it: %+v", p)
	}
	if v := ViewOf(inv, later); !v.Devices[0].New || v.Devices[0].MAC != macOther {
		t.Errorf("and it is listed first: %+v", v.Devices)
	}
}

func TestObserve_AFirstLookWithAnIPv6OnlyDeviceTreatsItAsBaseline(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, withV6(table("", v4("192.168.1.20", macTV)), nb6("fe80::9", macOther, "wlp2s0")), anywhere, t0)
	if d := find(inv, macOther); d == nil || !d.Approved {
		t.Errorf("device = %+v", d)
	}
}

func TestObserve_IPv6AddressesAreBoundedAndNewestFirst(t *testing.T) {
	inv := Observe(models.DeviceInventory{}, table("", v4("192.168.1.20", macTV)), anywhere, t0)
	for i := 0; i < maxIPv6PerDevice+4; i++ {
		addr := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 15: byte(i + 1)}).String()
		inv = Observe(inv, withV6(table("", v4("192.168.1.20", macTV)), nb6(addr, macTV, "wlp2s0")), anywhere, t0.Add(time.Duration(i+1)*time.Hour))
	}
	d := find(inv, macTV)
	if len(d.IPv6) != maxIPv6PerDevice || d.IPv6[0] != netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 15: byte(maxIPv6PerDevice + 4)}).String() {
		t.Errorf("addresses = %v", d.IPv6)
	}
}

func TestInventory_PendingNamesWhereTheNewDevicesAreAndMarksPrivateAddresses(t *testing.T) {
	st := &memInventory{}
	v := newInventory(fakeSource{table: table("", nb("192.168.1.1", macRouter))}, st)
	_, _ = v.Refresh(context.Background())
	v.src = fakeSource{table: withV6(table("", nb("192.168.1.1", macRouter), nb("192.168.1.77", macTV), nb("192.168.1.78", macPhone)), nb6("fe80::9", macOther, ""))}
	_, _ = v.Refresh(context.Background())
	p, err := v.Pending()
	if err != nil {
		t.Fatal(err)
	}
	msg := PresenceMessage(p)
	for _, want := range []string{"192.168.1.77", "192.168.1.78 (private address)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, macTV) || strings.Contains(msg, "Living") {
		t.Errorf("no hardware addresses or names in the message: %q", msg)
	}
}
