package stackport

import (
	"context"
	"net"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

const (
	// lanRouterSpread is how far either side of the gateway's address a router's Wi-Fi radios are looked for (Aruba's "minus one" and
	// AirDefense's BSSID range are the same idea, ADR-0058); lanMaxDevices bounds one answer.
	lanRouterSpread = 8
	lanMaxDevices   = 1024
	laBit           = 0x02
)

var _ stacksdk.LAN = LAN{}

// LAN is the SDK's LAN port over the core's network inventory (ADR-0058). It tags each device's hardware address with the vault, under the
// purpose the stack asked for, and hands back the tags and the operator's names: the stack can match them to the tags it makes itself, and no
// address crosses the port. The gateway is given with the addresses next to it, plain and with the locally administered bit flipped, because
// that is where a router's radios sit.
type LAN struct {
	Vault     stacksdk.Vault
	Inventory func() (models.DeviceInventory, error)
}

// Devices answers the inventory as tags under purpose.
func (l LAN) Devices(ctx context.Context, purpose string) ([]stacksdk.LANDevice, error) {
	if l.Vault == nil || l.Inventory == nil {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "the network inventory is not available")
	}
	inv, err := l.Inventory()
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "the network inventory could not be read")
	}
	out := []stacksdk.LANDevice{}
	for _, d := range inv.Devices {
		if len(out) >= lanMaxDevices {
			break
		}
		mac, err := net.ParseMAC(d.MAC)
		if err != nil || len(mac) != 6 {
			continue
		}
		tag, err := l.Vault.Tag(ctx, purpose, mac)
		if err != nil {
			return nil, err
		}
		out = append(out, stacksdk.LANDevice{Tag: tag, Name: d.Name, Randomized: d.Randomized})
	}
	return l.withRouter(ctx, purpose, inv, out)
}

// withRouter adds the gateway's address and its neighbours, marked Router.
func (l LAN) withRouter(ctx context.Context, purpose string, inv models.DeviceInventory, out []stacksdk.LANDevice) ([]stacksdk.LANDevice, error) {
	gw := inv.GatewayMAC
	if gw == "" {
		gw = inv.GatewaySeen
	}
	mac, err := net.ParseMAC(gw)
	if err != nil || len(mac) != 6 {
		return out, nil
	}
	name := nameOf(inv, gw)
	for _, flip := range []byte{0, laBit} {
		for off := -lanRouterSpread; off <= lanRouterSpread; off++ {
			m := offsetMAC(mac, off)
			m[0] ^= flip
			tag, err := l.Vault.Tag(ctx, purpose, m)
			if err != nil {
				return nil, err
			}
			out = append(out, stacksdk.LANDevice{Tag: tag, Name: name, Router: true, Offset: off})
		}
	}
	return out, nil
}

// offsetMAC is the address off steps away, counting over the last three bytes (the device part of the address block).
func offsetMAC(mac net.HardwareAddr, off int) net.HardwareAddr {
	v := int(mac[3])<<16 | int(mac[4])<<8 | int(mac[5])
	v = (v + off) & 0xffffff
	return net.HardwareAddr{mac[0], mac[1], mac[2], byte(v >> 16), byte(v >> 8), byte(v)}
}

func nameOf(inv models.DeviceInventory, mac string) string {
	want, err := net.ParseMAC(mac)
	if err != nil {
		return ""
	}
	for _, d := range inv.Devices {
		if m, err := net.ParseMAC(d.MAC); err == nil && m.String() == want.String() {
			return d.Name
		}
	}
	return ""
}
