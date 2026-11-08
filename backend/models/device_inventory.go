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
	// Baselined6 is true once a look that ran active IPv6 discovery has been taken; that look approves the IPv6-only devices
	// it found, as Baselined does for the first look.
	Baselined6 bool `json:"baselined6,omitempty"`
	// Gateway is the router's address, from the default route.
	Gateway string `json:"gateway,omitempty"`
	// GatewayMAC is the router's MAC address as last approved. A different one now is the signature of ARP spoofing.
	GatewayMAC string `json:"gateway_mac,omitempty"`
	// GatewaySeen is the router's MAC address as last read, and GatewayAlerted the one for which the operator has already
	// been told that it differs from GatewayMAC.
	GatewaySeen    string `json:"gateway_seen,omitempty"`
	GatewayAlerted string `json:"gateway_alerted,omitempty"`
	// Shared are the MAC addresses that answered for more than one IP address at the last look.
	Shared []SharedMAC `json:"shared,omitempty"`
	// SharedAlerted is a digest of the Shared set the operator was last told about, so the same set is told once; it is cleared when
	// no hardware address is shared, so the same thing happening again is told again.
	SharedAlerted string    `json:"shared_alerted,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NetworkDevice is one MAC address seen on the network.
type NetworkDevice struct {
	MAC string `json:"mac"`
	// IP is the IPv4 address it had when last seen; empty for a device only ever seen over IPv6. Every finding is keyed by
	// it, so a device without one cannot have a finding.
	IP string `json:"ip,omitempty"`
	// IPv6 are the IPv6 addresses it was seen with, newest first. Privacy addresses change every day, so they are shown
	// and never used to key anything.
	IPv6 []string `json:"ipv6,omitempty"`
	// Name is what the operator calls it; it is shown on screen only and never put in a finding, a prompt or a message.
	Name      string    `json:"name,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// Approved means the operator (or the first baseline) vouches for the device.
	Approved bool `json:"approved"`
	// Important means the operator marked it as one they care most about; the attack paths treat it as a prize.
	Important bool `json:"important,omitempty"`
	// Alerted means the operator has already been told this device is new.
	Alerted bool `json:"alerted,omitempty"`
	// Randomized is true for a locally administered MAC, which phones and laptops use to hide their hardware address.
	Randomized bool `json:"randomized,omitempty"`
	// UDPPorts and UDPScannedAt are the last UDP scan the operator ran on it through the privileged helper; the time is set even when
	// nothing answered, so "scanned, nothing found" and "never scanned" differ. OSGuesses and OSDetectedAt are the same for OS detection.
	// They are what that scan found then, shown on screen only: no finding is made from them.
	UDPPorts     []ProbeUDPPort `json:"udp_ports,omitempty"`
	UDPScannedAt time.Time      `json:"udp_scanned_at,omitzero"`
	OSGuesses    []ProbeOSGuess `json:"os_guesses,omitempty"`
	OSDetectedAt time.Time      `json:"os_detected_at,omitzero"`
}

// ProbeUDPPort is one UDP port a helper scan found. State "open|filtered" means no reply came back: it is never shown as open.
type ProbeUDPPort struct {
	Port    int    `json:"port"`
	State   string `json:"state"`
	Service string `json:"service,omitempty"`
}

// ProbeOSGuess is one of nmap's operating-system guesses with the accuracy it reported (0 to 100).
type ProbeOSGuess struct {
	Name     string `json:"name"`
	Accuracy int    `json:"accuracy"`
}

// DeviceProbe is what one helper scan of a device found, parsed and bounded by the service: UDP ports from a UDP scan, guesses from OS detection.
type DeviceProbe struct {
	UDPPorts []ProbeUDPPort `json:"udp_ports"`
	OS       []ProbeOSGuess `json:"os"`
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
