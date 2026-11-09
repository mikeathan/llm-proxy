package netaudit

import (
	"slices"
	"time"

	"llm-proxy/models"
)

// DeviceView is one device as the screen shows it.
type DeviceView struct {
	MAC  string   `json:"mac"`
	IP   string   `json:"ip,omitempty"`
	IPv6 []string `json:"ipv6,omitempty"`
	Name string   `json:"name,omitempty"`
	// Vendor is the maker of the network card, read from the bundled IEEE registry by the first three bytes of the address. It is
	// empty for a randomized address and for a prefix the registry does not list. It never comes from the device.
	Vendor string `json:"vendor,omitempty"`
	// SuggestedName is what the device called itself when asked, offered for the operator to confirm. It is shown on screen
	// only, and only while the device has no name of its own from the operator.
	SuggestedName string    `json:"suggested_name,omitempty"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	Approved      bool      `json:"approved"`
	Important     bool      `json:"important,omitempty"`
	Randomized    bool      `json:"randomized"`
	// New is true while the device is unapproved and was seen lately: the same test that makes it a finding.
	New bool `json:"new"`
	// OpenPorts are the ports the device had open at its last completed scan, and PortsScannedAt when that was; both are
	// empty for a device no scan has finished looking at. They help tell an unnamed device apart and are shown on screen only.
	OpenPorts      []int     `json:"open_ports,omitempty"`
	PortsScannedAt time.Time `json:"ports_scanned_at,omitzero"`
	// UDPPorts, UDPScannedAt, OSGuesses and OSDetectedAt are the last results of the scans the operator ran through the privileged helper;
	// the times are set even for a scan that found nothing. Shown on screen only.
	UDPPorts     []models.ProbeUDPPort `json:"udp_ports,omitempty"`
	UDPScannedAt time.Time             `json:"udp_scanned_at,omitzero"`
	OSGuesses    []models.ProbeOSGuess `json:"os_guesses,omitempty"`
	OSDetectedAt time.Time             `json:"os_detected_at,omitzero"`
}

// GatewayView is the router's hardware address and whether it is the one on record.
type GatewayView struct {
	IP      string `json:"ip,omitempty"`
	MAC     string `json:"mac,omitempty"`
	Changed bool   `json:"changed"`
}

// InventoryView is the device inventory as the screen shows it. Devices that need a look come first, then the rest by name
// and address.
type InventoryView struct {
	Devices   []DeviceView `json:"devices"`
	Gateway   GatewayView  `json:"gateway"`
	Baselined bool         `json:"baselined"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// VendorLookup names the maker of a network card from its hardware address, or returns "" when it does not know. The registry of
// makers belongs to the Network stack, so the composition root hands the core this function; a nil lookup names no maker.
type VendorLookup func(mac string) string

func (v VendorLookup) of(mac string) string {
	if v == nil {
		return ""
	}
	return v(mac)
}

// ViewOf builds the view of an inventory at a given time; vendors names the maker of each card.
func ViewOf(inv models.DeviceInventory, now time.Time, vendors VendorLookup) InventoryView {
	view := InventoryView{
		Devices:   make([]DeviceView, 0, len(inv.Devices)),
		Gateway:   GatewayView{IP: inv.Gateway, MAC: inv.GatewaySeen, Changed: gatewayChanged(inv)},
		Baselined: inv.Baselined,
		UpdatedAt: inv.UpdatedAt,
	}
	for _, d := range inv.Devices {
		view.Devices = append(view.Devices, DeviceView{
			MAC: d.MAC, IP: d.IP, IPv6: d.IPv6, Name: d.Name, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen,
			Approved: d.Approved, Important: d.Important, Randomized: d.Randomized, New: newDeviceIsNews(d, now), Vendor: vendors.of(d.MAC),
			UDPPorts: d.UDPPorts, UDPScannedAt: d.UDPScannedAt, OSGuesses: d.OSGuesses, OSDetectedAt: d.OSDetectedAt,
		})
	}
	slices.SortStableFunc(view.Devices, func(a, b DeviceView) int {
		if a.New != b.New {
			if a.New {
				return -1
			}
			return 1
		}
		return b.LastSeen.Compare(a.LastSeen)
	})
	return view
}
