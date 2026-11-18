package stackport

import (
	"context"
	"net"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

var _ stacksdk.MeshClients = Mesh{}

// Mesh is the SDK's meshclients port over the core's read of the mesh router's list of joined devices (Constitution I.15, ADR-0059). It tags
// each device's and each box's hardware address with the vault under the purpose the stack asked for, names each box from the network
// inventory (the gateway is the main box), and hands back no address. The read itself, and its limits, are the core's: Read is the router
// manager's MeshClients, wired in internal/app, so this package never reaches the router.
type Mesh struct {
	Vault     stacksdk.Vault
	Read      func(context.Context) (*models.MeshClients, error)
	Inventory func() (models.DeviceInventory, error)
}

// Clients answers the latest list as tags under purpose.
func (m Mesh) Clients(ctx context.Context, purpose string) (stacksdk.MeshView, error) {
	if m.Vault == nil || m.Read == nil {
		return stacksdk.MeshView{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the router's list of devices is not available")
	}
	list, err := m.Read(ctx)
	if err != nil || list == nil {
		return stacksdk.MeshView{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the router's list of devices could not be read")
	}
	names, gateway := m.boxNames()
	view := stacksdk.MeshView{At: list.At, Devices: make([]stacksdk.MeshDevice, 0, len(list.Clients))}
	boxTags := map[string][]byte{}
	for _, c := range list.Clients {
		tag, err := m.tag(ctx, purpose, c.MAC)
		if err != nil {
			return stacksdk.MeshView{}, err
		}
		box, ok := boxTags[c.Box]
		if !ok {
			if box, err = m.tag(ctx, purpose, c.Box); err != nil {
				return stacksdk.MeshView{}, err
			}
			boxTags[c.Box] = box
		}
		view.Devices = append(view.Devices, stacksdk.MeshDevice{Tag: tag, Box: box, BoxName: names[c.Box], Main: c.Box == gateway,
			Band: c.Band, Guest: c.Guest, RSSI: c.RSSI})
	}
	return view, nil
}

func (m Mesh) tag(ctx context.Context, purpose, mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "a device in the router's list has no usable address")
	}
	return m.Vault.Tag(ctx, purpose, hw)
}

// boxNames is the operator's name for each device the inventory knows, by lower-case hardware address, and the gateway's address.
func (m Mesh) boxNames() (map[string]string, string) {
	names := map[string]string{}
	if m.Inventory == nil {
		return names, ""
	}
	inv, err := m.Inventory()
	if err != nil {
		return names, ""
	}
	for _, d := range inv.Devices {
		if d.Name != "" {
			names[canonical(d.MAC)] = d.Name
		}
	}
	return names, canonical(inv.GatewayMAC)
}

// canonical is a hardware address as the router's list is read (lower case, colons), empty when it is not one.
func canonical(s string) string {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return ""
	}
	return strings.ToLower(hw.String())
}
