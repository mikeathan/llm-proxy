package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"llm-proxy/models"
)

// Checks the device inventory produces. They are judged from the host's neighbour table and the inventory it keeps, in
// fixed words: nothing a device sent, and no name the operator typed, is in a finding.
const (
	CheckNewDevice      = "new_device"
	CheckMACChanged     = "mac_changed"
	CheckGatewayChanged = "gateway_mac_changed"
	CheckDuplicateMAC   = "duplicate_mac"
	CheckDeviceMissing  = "device_missing"
	CheckHiddenDevice   = "hidden_device"
)

const (
	// InventoryActiveWindow is how long a device that nobody approved is reported after it was last seen: a visitor's phone
	// that left a week ago is no longer news. It stays in the list.
	InventoryActiveWindow = 7 * 24 * time.Hour
	// ChangeWindow is how long a change of address owner is kept and reported.
	ChangeWindow = 7 * 24 * time.Hour
	// MissingAfter is how long a device the operator named may stay unseen before it is reported: three nights, so a laptop
	// closed for a weekend is not a finding.
	MissingAfter = 3 * 24 * time.Hour

	maxInventoryDevices = 1024
	maxIPv6PerDevice    = 6
	maxAddressChanges   = 64
	maxDeviceNameLen    = 40
	dateLayout          = "2006-01-02"
)

var (
	// ErrBadMAC means a hardware address is not six colon-separated bytes.
	ErrBadMAC = errors.New("that is not a hardware (MAC) address")
	// ErrBadDeviceName means a device name is empty-looking, too long or has control characters.
	ErrBadDeviceName = errors.New("a device name is at most 40 characters, without control characters")
	// ErrNoSuchDevice means the device is not in the inventory.
	ErrNoSuchDevice = errors.New("no such device in the inventory")
)

// Neighbor is one entry of the host's neighbour table: a hardware address that answered for an IPv4 address.
type Neighbor struct {
	Addr netip.Addr
	// MAC is lower case and colon separated.
	MAC string
	// Interface is the network interface the entry was learned on, used to tell which IPv6 neighbours share a segment with
	// the IPv4 devices in scope.
	Interface string
}

// NeighborTable is a read of the host's neighbour table.
type NeighborTable struct {
	Entries []Neighbor
	// Entries6 are the IPv6 neighbours the host knows. The host lists only devices it has talked to, so this is sparse.
	Entries6 []Neighbor
	// Gateway is the default route's next hop, or the zero Addr when there is none.
	Gateway netip.Addr
}

// NeighborSource reads the neighbour table. An error means the host could not look, which is not the same as nobody being
// there.
type NeighborSource interface {
	Neighbors(ctx context.Context) (NeighborTable, error)
}

// randomized reports whether a MAC has the locally administered bit set, which is how a phone hides its hardware address.
func randomized(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) > 0 && hw[0]&2 == 2
}

// Observe folds one read of the neighbour table into the inventory. It is pure: the same table at the same time gives the
// same result. Only addresses the scan scope allows are kept, so a container bridge on the host is not part of the network.
// The first look that finds anything is a baseline: everything it finds is approved. After that a hardware address not seen
// before is a new, unapproved device; an address that a different hardware address now answers for is recorded as a change;
// the router's hardware address is compared with the one on record.
func Observe(inv models.DeviceInventory, table NeighborTable, inScope func(netip.Addr) bool, at time.Time) models.DeviceInventory {
	entries := scopedEntries(table.Entries, inScope)
	owners := lastOwners(inv.Devices)
	index := map[string]int{}
	devices := slices.Clone(inv.Devices)
	for i, d := range devices {
		index[d.MAC] = i
	}
	out := inv
	changes := slices.Clone(inv.Changes)
	for _, e := range entries {
		ip := e.Addr.String()
		if prior, held := owners[ip]; held && prior != e.MAC && inv.Baselined {
			changes = append(changes, models.AddressChange{IP: ip, From: prior, To: e.MAC, At: at})
		}
		if i, known := index[e.MAC]; known {
			devices[i].IP, devices[i].LastSeen = ip, at
			continue
		}
		index[e.MAC] = len(devices)
		devices = append(devices, models.NetworkDevice{
			MAC: e.MAC, IP: ip, FirstSeen: at, LastSeen: at, Approved: !inv.Baselined, Randomized: randomized(e.MAC),
		})
	}
	devices = attachIPv6(devices, scoped6(table.Entries6, entries), inv.Baselined, at)
	out.Devices = capDevices(devices)
	out.Changes = recentChanges(changes, at)
	out.Shared = sharedMACs(entries, table.Gateway)
	observeGateway(&out, entries, table.Gateway)
	if len(entries) > 0 {
		out.Baselined = true
	}
	out.UpdatedAt = at
	return out
}

// scoped6 keeps the IPv6 neighbours that share a network interface with an in-scope IPv4 device: the declared scan networks
// are IPv4, so the interface is what says an IPv6 neighbour is on the same segment and not on a container bridge.
func scoped6(v6, v4 []Neighbor) []Neighbor {
	segments := map[string]bool{}
	for _, e := range v4 {
		if e.Interface != "" {
			segments[e.Interface] = true
		}
	}
	var out []Neighbor
	for _, e := range v6 {
		if e.Addr.Is6() && segments[e.Interface] && !e.Addr.IsLoopback() && !e.Addr.IsMulticast() {
			out = append(out, e)
		}
	}
	return out
}

// attachIPv6 puts each IPv6 address beside the device with the same hardware address, newest first and bounded. A hardware
// address seen only over IPv6 becomes a device without an IPv4 address: it is listed and reported to the operator, but no
// finding is keyed by an IPv6 address, because privacy addresses change daily.
func attachIPv6(devices []models.NetworkDevice, v6 []Neighbor, baselined bool, at time.Time) []models.NetworkDevice {
	index := map[string]int{}
	for i, d := range devices {
		index[d.MAC] = i
	}
	for _, e := range v6 {
		i, known := index[e.MAC]
		if !known {
			index[e.MAC] = len(devices)
			devices = append(devices, models.NetworkDevice{MAC: e.MAC, FirstSeen: at, LastSeen: at, Approved: !baselined, Randomized: randomized(e.MAC)})
			i = len(devices) - 1
		}
		d := &devices[i]
		d.LastSeen = at
		d.IPv6 = dedupeKeepFirst(slices.Insert(slices.Clone(d.IPv6), 0, e.Addr.String()))
		if len(d.IPv6) > maxIPv6PerDevice {
			d.IPv6 = d.IPv6[:maxIPv6PerDevice]
		}
	}
	return devices
}

func dedupeKeepFirst(list []string) []string {
	seen := map[string]bool{}
	out := list[:0]
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func scopedEntries(entries []Neighbor, inScope func(netip.Addr) bool) []Neighbor {
	var out []Neighbor
	seen := map[netip.Addr]bool{}
	for _, e := range entries {
		if e.Addr.IsValid() && !seen[e.Addr] && inScope(e.Addr) {
			seen[e.Addr] = true
			out = append(out, e)
		}
	}
	return out
}

// lastOwners maps each address to the hardware address that last held it, the most recently seen device winning.
func lastOwners(devices []models.NetworkDevice) map[string]string {
	owners := map[string]string{}
	newest := map[string]time.Time{}
	for _, d := range devices {
		if d.IP == "" {
			continue
		}
		if seen, ok := newest[d.IP]; !ok || d.LastSeen.After(seen) {
			owners[d.IP], newest[d.IP] = d.MAC, d.LastSeen
		}
	}
	return owners
}

// capDevices keeps the inventory bounded: when it is full, the devices seen longest ago go first, unnamed before named.
func capDevices(devices []models.NetworkDevice) []models.NetworkDevice {
	if len(devices) <= maxInventoryDevices {
		return devices
	}
	slices.SortStableFunc(devices, func(a, b models.NetworkDevice) int {
		if (a.Name != "") != (b.Name != "") {
			if a.Name != "" {
				return -1
			}
			return 1
		}
		return b.LastSeen.Compare(a.LastSeen)
	})
	return devices[:maxInventoryDevices]
}

func recentChanges(changes []models.AddressChange, at time.Time) []models.AddressChange {
	var out []models.AddressChange
	for _, c := range changes {
		if at.Sub(c.At) <= ChangeWindow {
			out = append(out, c)
		}
	}
	if len(out) > maxAddressChanges {
		out = out[len(out)-maxAddressChanges:]
	}
	return out
}

// sharedMACs lists the hardware addresses answering for several addresses at once, other than the router's, which may
// legitimately have more than one.
func sharedMACs(entries []Neighbor, gateway netip.Addr) []models.SharedMAC {
	byMAC := map[string][]string{}
	gatewayMAC := macOf(entries, gateway)
	for _, e := range entries {
		byMAC[e.MAC] = append(byMAC[e.MAC], e.Addr.String())
	}
	var out []models.SharedMAC
	for mac, ips := range byMAC {
		if len(ips) > 1 && mac != gatewayMAC {
			slices.Sort(ips)
			out = append(out, models.SharedMAC{MAC: mac, IPs: ips})
		}
	}
	slices.SortFunc(out, func(a, b models.SharedMAC) int { return strings.Compare(a.MAC, b.MAC) })
	return out
}

func macOf(entries []Neighbor, addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	for _, e := range entries {
		if e.Addr == addr {
			return e.MAC
		}
	}
	return ""
}

// observeGateway records the router's hardware address. The first one seen becomes the approved one; a different one later
// is kept apart from it until the operator confirms it, so the network cannot make a spoofed router the new normal.
func observeGateway(inv *models.DeviceInventory, entries []Neighbor, gateway netip.Addr) {
	mac := macOf(entries, gateway)
	if mac == "" {
		return
	}
	inv.Gateway, inv.GatewaySeen = gateway.String(), mac
	if inv.GatewayMAC == "" {
		inv.GatewayMAC = mac
	}
}

// ConfirmGateway accepts the router's current hardware address as the right one.
func ConfirmGateway(inv models.DeviceInventory) models.DeviceInventory {
	if inv.GatewaySeen != "" {
		inv.GatewayMAC = inv.GatewaySeen
	}
	return inv
}

func deviceByMAC(inv models.DeviceInventory, mac string) *models.NetworkDevice {
	for i := range inv.Devices {
		if inv.Devices[i].MAC == mac {
			return &inv.Devices[i]
		}
	}
	return nil
}

// newDeviceIsNews reports whether an unapproved device should still be reported.
func newDeviceIsNews(d models.NetworkDevice, now time.Time) bool {
	return !d.Approved && now.Sub(d.LastSeen) <= InventoryActiveWindow
}

// changeIsNotable reports whether a change of address owner is worth reporting: a device the operator vouched for lost its
// address to one nobody has. Two approved devices trading a lease is ordinary.
func changeIsNotable(inv models.DeviceInventory, c models.AddressChange, now time.Time) bool {
	from, to := deviceByMAC(inv, c.From), deviceByMAC(inv, c.To)
	return now.Sub(c.At) <= ChangeWindow && from != nil && from.Approved && to != nil && !to.Approved
}

func gatewayChanged(inv models.DeviceInventory) bool {
	return inv.GatewayMAC != "" && inv.GatewaySeen != "" && inv.GatewaySeen != inv.GatewayMAC
}

// InventoryFindings turns the inventory into findings, all keyed by the IP address like every other, so accepting a finding,
// the baseline and the report work unchanged. They are derived from what the inventory holds now, so the nightly report
// shows what the presence check noticed in between.
func InventoryFindings(inv models.DeviceInventory, now time.Time) []Finding {
	var out []Finding
	out = append(out, newDeviceFindings(inv, now)...)
	for _, c := range inv.Changes {
		if changeIsNotable(inv, c, now) {
			out = append(out, Finding{Device: c.IP, Check: CheckMACChanged, Severity: SeverityMedium,
				Evidence: fmt.Sprintf("A device you approved had this address, and a different one has it now (changed %s). If the first did not simply go away, someone may be pretending to be it", c.At.Format(dateLayout))})
		}
	}
	if gatewayChanged(inv) {
		out = append(out, Finding{Device: inv.Gateway, Check: CheckGatewayChanged, Severity: SeverityCritical,
			Evidence: "The router's hardware address is not the one on record. Something else may be answering for the router, which would let it read or change the network's traffic. A replaced or reset router also does this"})
	}
	for _, s := range inv.Shared {
		for _, ip := range s.IPs {
			out = append(out, Finding{Device: ip, Check: CheckDuplicateMAC, Severity: SeverityMedium,
				Evidence: fmt.Sprintf("One device answers for %d addresses at once. That is normal for a machine with several network connections and a sign of impersonation otherwise", len(s.IPs))})
		}
	}
	for _, d := range inv.Devices {
		if d.IP != "" && d.Name != "" && !d.Randomized && !d.LastSeen.IsZero() && now.Sub(d.LastSeen) > MissingAfter {
			out = append(out, Finding{Device: d.IP, Check: CheckDeviceMissing, Severity: SeverityLow,
				Evidence: fmt.Sprintf("A device you named has not been seen on the network for %d days (last seen %s)", int(now.Sub(d.LastSeen).Hours()/24), d.LastSeen.Format(dateLayout))})
		}
	}
	return out
}

func newDeviceFindings(inv models.DeviceInventory, now time.Time) []Finding {
	var out []Finding
	for _, d := range inv.Devices {
		if d.IP == "" || !newDeviceIsNews(d, now) {
			continue
		}
		f := Finding{Device: d.IP, Check: CheckNewDevice, Severity: SeverityMedium,
			Evidence: fmt.Sprintf("A device that was not on the network before appeared (first seen %s)", d.FirstSeen.Format(dateLayout))}
		if d.Randomized {
			f.Severity = SeverityLow
			f.Evidence += ". It uses a private hardware address, as phones and laptops often do, so it may be a visitor's"
		}
		out = append(out, f)
	}
	return out
}

// HiddenDeviceFindings reports a device that answered for its address on the network but on which the scan found nothing
// to talk to: no port answered and none refused. A phone asleep does this, so one with a private hardware address is left
// out; a machine that is awake and says nothing to anyone is worth a look.
func HiddenDeviceFindings(current []Neighbor, hosts []Host) []Finding {
	swept := map[netip.Addr]Host{}
	for _, h := range hosts {
		swept[h.Addr] = h
	}
	var out []Finding
	for _, n := range current {
		if h, ok := swept[n.Addr]; ok && !h.Up && !h.Unscanned && !randomized(n.MAC) {
			out = append(out, Finding{Device: n.Addr.String(), Check: CheckHiddenDevice, Severity: SeverityLow,
				Evidence: "This address answers on the network but no port answered the scan, not even with a refusal. A computer that is awake normally refuses; one that says nothing may be hiding"})
		}
	}
	return out
}

// InventoryStore keeps the inventory. A missing file is an empty inventory.
type InventoryStore interface {
	Read() (models.DeviceInventory, error)
	Write(models.DeviceInventory) error
}

// Snapshot is the result of one look at the network.
type Snapshot struct {
	Inventory models.DeviceInventory
	// Current are the entries of the neighbour table that were in scope.
	Current []Neighbor
}

// Inventory is the service over the stored inventory. The nightly audit and the presence check both go through it, so one
// look is never reported twice and two looks never overwrite each other.
type Inventory struct {
	mu      sync.Mutex
	store   InventoryStore
	src     NeighborSource
	inScope func(netip.Addr) bool
	now     func() time.Time
}

// NewInventory builds the service. inScope says which addresses belong to the declared scan scope.
func NewInventory(store InventoryStore, src NeighborSource, inScope func(netip.Addr) bool) *Inventory {
	return &Inventory{store: store, src: src, inScope: inScope, now: time.Now}
}

// Refresh reads the neighbour table and folds it into the stored inventory. If the table cannot be read it returns the
// error and changes nothing: a look that failed is not a look at an empty network.
func (v *Inventory) Refresh(ctx context.Context) (Snapshot, error) {
	table, err := v.src.Neighbors(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	inv, err := v.store.Read()
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to read the device inventory: %w", err)
	}
	next := Observe(inv, table, v.inScope, v.now())
	if err := v.store.Write(next); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Inventory: next, Current: scopedEntries(table.Entries, v.inScope)}, nil
}

// View returns the stored inventory.
func (v *Inventory) View() (models.DeviceInventory, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.store.Read()
}

// edit applies a change to the stored inventory under the lock.
func (v *Inventory) edit(change func(*models.DeviceInventory) error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	inv, err := v.store.Read()
	if err != nil {
		return fmt.Errorf("failed to read the device inventory: %w", err)
	}
	if err := change(&inv); err != nil {
		return err
	}
	return v.store.Write(inv)
}

// normalizeMAC checks and lower-cases a hardware address.
func normalizeMAC(mac string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil || len(hw) != 6 {
		return "", ErrBadMAC
	}
	return strings.ToLower(hw.String()), nil
}

// cleanName trims a device name and refuses one that is too long or holds control characters. An empty name is allowed: it
// removes the name.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > maxDeviceNameLen || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrBadDeviceName
	}
	return name, nil
}

// Name gives a device a name and approves it: the operator who names a device knows it.
func (v *Inventory) Name(mac, name string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	if name, err = cleanName(name); err != nil {
		return err
	}
	return v.edit(func(inv *models.DeviceInventory) error {
		d := deviceByMAC(*inv, mac)
		if d == nil {
			return ErrNoSuchDevice
		}
		d.Name, d.Approved = name, true
		return nil
	})
}

// Approve vouches for a device without naming it.
func (v *Inventory) Approve(mac string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	return v.edit(func(inv *models.DeviceInventory) error {
		d := deviceByMAC(*inv, mac)
		if d == nil {
			return ErrNoSuchDevice
		}
		d.Approved = true
		return nil
	})
}

// Forget removes a device from the inventory. If it is on the network it comes back as a new device.
func (v *Inventory) Forget(mac string) error {
	mac, err := normalizeMAC(mac)
	if err != nil {
		return err
	}
	return v.edit(func(inv *models.DeviceInventory) error {
		before := len(inv.Devices)
		inv.Devices = slices.DeleteFunc(inv.Devices, func(d models.NetworkDevice) bool { return d.MAC == mac })
		if len(inv.Devices) == before {
			return ErrNoSuchDevice
		}
		return nil
	})
}

// ConfirmGateway accepts the router's current hardware address as the right one.
func (v *Inventory) ConfirmGateway() error {
	return v.edit(func(inv *models.DeviceInventory) error {
		*inv = ConfirmGateway(*inv)
		return nil
	})
}

// Pending is what the operator has not yet been told: counts for the message, and the identities of what they stand for so
// that exactly those are marked once the message is sent.
type Pending struct {
	NewDevices int
	Changes    int
	Gateway    bool

	macs       []string
	changes    []models.AddressChange
	gatewayMAC string
}

// Empty reports whether there is nothing to tell.
func (p Pending) Empty() bool { return p.NewDevices == 0 && p.Changes == 0 && !p.Gateway }

// Pending lists what is new since the operator was last told. Nothing is marked: the caller marks it with MarkNotified once
// the message was actually sent, so a failed send is tried again.
func (v *Inventory) Pending() (Pending, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	inv, err := v.store.Read()
	if err != nil {
		return Pending{}, fmt.Errorf("failed to read the device inventory: %w", err)
	}
	return pendingOf(inv, v.now()), nil
}

func pendingOf(inv models.DeviceInventory, now time.Time) Pending {
	var p Pending
	for _, d := range inv.Devices {
		if newDeviceIsNews(d, now) && !d.Alerted {
			p.macs = append(p.macs, d.MAC)
		}
	}
	for _, c := range inv.Changes {
		if !c.Notified && changeIsNotable(inv, c, now) {
			p.changes = append(p.changes, c)
		}
	}
	if gatewayChanged(inv) && inv.GatewayAlerted != inv.GatewaySeen {
		p.Gateway, p.gatewayMAC = true, inv.GatewaySeen
	}
	p.NewDevices, p.Changes = len(p.macs), len(p.changes)
	return p
}

// MarkNotified records that the operator was told about exactly what p holds.
func (v *Inventory) MarkNotified(p Pending) error {
	return v.edit(func(inv *models.DeviceInventory) error {
		for _, mac := range p.macs {
			if d := deviceByMAC(*inv, mac); d != nil {
				d.Alerted = true
			}
		}
		for _, told := range p.changes {
			for i := range inv.Changes {
				if c := &inv.Changes[i]; c.IP == told.IP && c.From == told.From && c.To == told.To && c.At.Equal(told.At) {
					c.Notified = true
				}
			}
		}
		if p.Gateway {
			inv.GatewayAlerted = p.gatewayMAC
		}
		return nil
	})
}
