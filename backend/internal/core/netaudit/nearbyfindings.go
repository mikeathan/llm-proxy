package netaudit

import (
	"net/netip"

	"llm-proxy/models"
)

// CheckRogueGateway is a device on the network that says it is an Internet Gateway Device (it offers to open internet ports for
// others) and is not the router the device inventory found.
const CheckRogueGateway = "rogue_gateway"

// rogueGatewayWords is the finding's text. It is fixed: nothing the device said is in it, and it says what the device claims,
// because a device on the network can claim anything.
const rogueGatewayWords = "This device says it is an internet gateway, which means it offers to open internet ports for others, and it is not your router. A second gateway is what a misconfigured access point, a rogue box or a device pretending to be one looks like"

// RogueGatewayFindings reports each device that claims to be a gateway other than the router at gateway. With no known router
// nothing can be judged, so nothing is reported. The claim is read from the kept state, not from the latest look, so a look
// that heard nothing never resolves it: it goes only when a later answer no longer says gateway (Constitution I.9).
func RogueGatewayFindings(state models.NearbyState, gateway netip.Addr) []Finding {
	if !gateway.IsValid() {
		return nil
	}
	var out []Finding
	for _, d := range state.Devices {
		if d.Gateway && d.IP != gateway.String() {
			out = append(out, Finding{Device: d.IP, Check: CheckRogueGateway, Severity: SeverityMedium, Evidence: rogueGatewayWords})
		}
	}
	return out
}

// NearbyFindings reads the kept Nearby state and the router's address and turns them into findings.
type NearbyFindings struct {
	State   func() (models.NearbyState, error)
	Gateway func() netip.Addr
	// Refresh asks for a fresh look in the background, once per audit; it may be nil. The findings below are read from what is
	// kept, so a look that is still running never changes this audit's report.
	Refresh func()
}

// Findings returns the rogue-gateway findings. A state that cannot be read gives an error, never an empty answer.
func (n NearbyFindings) Findings() ([]Finding, error) {
	if n.Refresh != nil {
		n.Refresh()
	}
	state, err := n.State()
	if err != nil {
		return nil, err
	}
	return RogueGatewayFindings(state, n.Gateway()), nil
}
