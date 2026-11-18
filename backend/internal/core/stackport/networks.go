package stackport

import (
	"bytes"
	"context"
	"net"
	"slices"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

var _ stacksdk.OwnNetworks = Networks{}

// Networks is the SDK's ownnetworks port over the core's one list of the owner's Wi-Fi networks (models.NetauditConfig.WifiNetworks,
// ADR-0063). It answers the names and, as tags under the purpose the stack asked for, the access points the owner confirmed; it remembers each
// purpose asked, and it is the one place that checks what the stack reports back about access points (Sightings). The list is changed only by
// the operator on the Network page: nothing here writes it.
type Networks struct {
	Vault stacksdk.Vault
	List  func() []models.WifiNetwork
	// Asked remembers the purposes the stack asked for; Sightings opens nothing under any other.
	Asked *AskedPurposes
}

// Networks answers the owner's networks as names and tags under purpose.
func (n Networks) Networks(ctx context.Context, purpose string) ([]stacksdk.OwnNetwork, error) {
	if n.Vault == nil || n.List == nil {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "the owner's Wi-Fi networks are not available")
	}
	n.Asked.remember(purpose)
	list := n.List()
	out := make([]stacksdk.OwnNetwork, 0, min(len(list), stacksdk.MaxOwnNetworks))
	for _, w := range list[:min(len(list), stacksdk.MaxOwnNetworks)] {
		on := stacksdk.OwnNetwork{Name: w.Name, Confirmed: [][]byte{}}
		for _, a := range w.Addresses {
			mac, err := net.ParseMAC(a)
			if err != nil || len(mac) != 6 {
				continue
			}
			tag, err := n.Vault.Tag(ctx, purpose, mac)
			if err != nil {
				return nil, err
			}
			on.Confirmed = append(on.Confirmed, tag)
		}
		out = append(out, on)
	}
	return out, nil
}

// Sightings checks the access points a stack reported and returns those that pass, at most MaxAirAPs. An entry passes only when its name is
// one of the owner's names byte for byte, its sealed address opens under a purpose the stack asked this port for, the address is one unicast
// hardware address, its tag under that purpose is the entry's tag, and its security is a known word (else it is "other"). Everything else is
// dropped: a stack can tell the core what it heard about the owner's own names, and nothing more.
func (n Networks) Sightings(ctx context.Context, aps []stacksdk.AirAP) []models.WifiSighting {
	if n.Vault == nil || n.List == nil {
		return nil
	}
	names := map[string]bool{}
	for _, w := range n.List() {
		names[w.Name] = true
	}
	purposes := n.Asked.List()
	out := []models.WifiSighting{}
	for _, a := range aps[:min(len(aps), stacksdk.MaxAirAPs)] {
		if !names[a.Name] {
			continue
		}
		mac, ok := n.open(ctx, purposes, a)
		if !ok {
			continue
		}
		out = append(out, models.WifiSighting{Name: a.Name, BSSID: strings.ToLower(mac.String()), Security: sightingSecurity(a.Security),
			Channel: max(a.Channel, 0), Heard: a.Heard.UTC(), ConfirmedBefore: a.ConfirmedBefore})
	}
	return out
}

// open finds the purpose, among those handed out, under which the entry's sealed address opens to a unicast hardware address whose tag is the
// entry's tag.
func (n Networks) open(ctx context.Context, purposes []string, a stacksdk.AirAP) (net.HardwareAddr, bool) {
	if len(a.Tag) == 0 || len(a.Sealed) == 0 {
		return nil, false
	}
	for _, p := range purposes {
		raw, err := n.Vault.Open(ctx, p, a.Sealed)
		if err != nil || len(raw) != 6 || raw[0]&0x01 != 0 {
			continue
		}
		mac := net.HardwareAddr(raw)
		if tag, err := n.Vault.Tag(ctx, p, mac); err == nil && bytes.Equal(tag, a.Tag) {
			return mac, true
		}
	}
	return nil, false
}

// sightingSecurity maps the stack's word to the look-alike check's: a network that offers both WPA2 and WPA3 is judged by the weaker, and
// anything the check does not rank is "other", never weak.
func sightingSecurity(s string) string {
	switch {
	case s == "wpa2_wpa3":
		return "wpa2"
	case slices.Contains([]string{"open", "wep", "wpa", "wpa2", "wpa3"}, s):
		return s
	default:
		return "other"
	}
}
