package netaudit

import (
	"net/netip"
	"slices"
	"time"

	"llm-proxy/models"
)

// NearbyProblem is a fixed sentence about a device and the guide that fixes it. Nothing the device said is in it.
type NearbyProblem struct {
	Check string    `json:"check"`
	Title string    `json:"title"`
	Text  string    `json:"text"`
	Guide *FixGuide `json:"guide,omitempty"`
}

// NearbyDeviceView is one device as the Nearby card shows it: what it said about itself, joined with what the device list knows
// about the same address.
type NearbyDeviceView struct {
	models.NearbyDevice
	// MAC and Name come from the device list when it knows this address; Known is false for a device it has not seen.
	MAC   string `json:"mac,omitempty"`
	Name  string `json:"name,omitempty"`
	Known bool   `json:"known"`
	// New is the device list's own flag (unapproved and seen lately), not a second definition.
	New     bool           `json:"new"`
	Problem *NearbyProblem `json:"problem,omitempty"`
}

// NearbyView is the Nearby card's data.
type NearbyView struct {
	Enabled     bool               `json:"enabled"`
	Running     bool               `json:"running"`
	LastLook    time.Time          `json:"last_look,omitzero"`
	LastOutcome string             `json:"last_outcome,omitempty"`
	Devices     []NearbyDeviceView `json:"devices"`
}

// BuildNearbyView joins the kept state with the device list. Devices with a problem come first, then new ones, then the rest by
// how lately they were heard.
func BuildNearbyView(enabled, running bool, state models.NearbyState, devices []DeviceView, gateway netip.Addr) NearbyView {
	byIP := make(map[string]DeviceView, len(devices))
	for _, d := range devices {
		if d.IP != "" {
			byIP[d.IP] = d
		}
	}
	problems := map[string]bool{}
	for _, f := range RogueGatewayFindings(state, gateway) {
		problems[f.Device] = true
	}
	view := NearbyView{Enabled: enabled, Running: running, LastLook: state.LastLook, LastOutcome: state.LastOutcome, Devices: make([]NearbyDeviceView, 0, len(state.Devices))}
	for _, d := range state.Devices {
		v := NearbyDeviceView{NearbyDevice: d}
		if known, ok := byIP[d.IP]; ok {
			v.MAC, v.Name, v.Known, v.New = known.MAC, known.Name, true, known.New
		}
		if problems[d.IP] {
			guide, _ := GuideFor(CheckRogueGateway)
			v.Problem = &NearbyProblem{Check: CheckRogueGateway, Title: CheckTitle(CheckRogueGateway), Text: rogueGatewayWords, Guide: &guide}
		}
		view.Devices = append(view.Devices, v)
	}
	slices.SortStableFunc(view.Devices, func(a, b NearbyDeviceView) int {
		if (a.Problem != nil) != (b.Problem != nil) {
			if a.Problem != nil {
				return -1
			}
			return 1
		}
		if a.New != b.New {
			if a.New {
				return -1
			}
			return 1
		}
		return b.LastHeard.Compare(a.LastHeard)
	})
	return view
}
