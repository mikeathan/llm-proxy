package netaudit

import (
	"slices"
	"time"

	"llm-proxy/models"
)

// DeviceView is one device as the screen shows it.
type DeviceView struct {
	MAC        string    `json:"mac"`
	IP         string    `json:"ip,omitempty"`
	IPv6       []string  `json:"ipv6,omitempty"`
	Name       string    `json:"name,omitempty"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Approved   bool      `json:"approved"`
	Randomized bool      `json:"randomized"`
	// New is true while the device is unapproved and was seen lately: the same test that makes it a finding.
	New bool `json:"new"`
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

// ViewOf builds the view of an inventory at a given time.
func ViewOf(inv models.DeviceInventory, now time.Time) InventoryView {
	view := InventoryView{
		Devices:   make([]DeviceView, 0, len(inv.Devices)),
		Gateway:   GatewayView{IP: inv.Gateway, MAC: inv.GatewaySeen, Changed: gatewayChanged(inv)},
		Baselined: inv.Baselined,
		UpdatedAt: inv.UpdatedAt,
	}
	for _, d := range inv.Devices {
		view.Devices = append(view.Devices, DeviceView{
			MAC: d.MAC, IP: d.IP, IPv6: d.IPv6, Name: d.Name, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen,
			Approved: d.Approved, Randomized: d.Randomized, New: newDeviceIsNews(d, now),
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
