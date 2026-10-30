package models

import "time"

// DeviceInventory is who has been seen on the network: one entry per link-layer (MAC) address, kept for the whole
// application because the network is the host's, not a workspace's. It is written by the audit and by the presence check,
// never by a model.
type DeviceInventory struct {
	Devices []NetworkDevice `json:"devices"`
	// Changes are the addresses that changed hands lately, newest last; they are dropped after a while.
	Changes []AddressChange `json:"changes,omitempty"`
	// Baselined is true once a first look has been taken; that look makes every device it found approved, so a first audit
	// reports a baseline and not forty "new" devices.
	Baselined bool `json:"baselined"`
	// Gateway is the router's address, from the default route.
	Gateway string `json:"gateway,omitempty"`
	// GatewayMAC is the router's MAC address as last approved. A different one now is the signature of ARP spoofing.
	GatewayMAC string `json:"gateway_mac,omitempty"`
	// GatewaySeen is the router's MAC address as last read, and GatewayAlerted the one for which the operator has already
	// been told that it differs from GatewayMAC.
	GatewaySeen    string `json:"gateway_seen,omitempty"`
	GatewayAlerted string `json:"gateway_alerted,omitempty"`
	// Shared are the MAC addresses that answered for more than one IP address at the last look.
	Shared    []SharedMAC `json:"shared,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// NetworkDevice is one MAC address seen on the network.
type NetworkDevice struct {
	MAC string `json:"mac"`
	// IP is the address it had when last seen.
	IP string `json:"ip"`
	// Name is what the operator calls it; it is shown on screen only and never put in a finding, a prompt or a message.
	Name      string    `json:"name,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// Approved means the operator (or the first baseline) vouches for the device.
	Approved bool `json:"approved"`
	// Alerted means the operator has already been told this device is new.
	Alerted bool `json:"alerted,omitempty"`
	// Randomized is true for a locally administered MAC, which phones and laptops use to hide their hardware address.
	Randomized bool `json:"randomized,omitempty"`
}

// AddressChange records that an address, which one MAC held, is now held by another.
type AddressChange struct {
	IP       string    `json:"ip"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	At       time.Time `json:"at"`
	Notified bool      `json:"notified,omitempty"`
}

// SharedMAC is one hardware address that answered for several IP addresses at once.
type SharedMAC struct {
	MAC string   `json:"mac"`
	IPs []string `json:"ips"`
}
