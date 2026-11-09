package models

import "time"

// CaptureSummary is what one capture_window run saw (plan network-capture-window-plan): typed counts of the broadcast and multicast frames
// that passed the helper's fixed filter, read from the first 64 bytes of each. Nothing a device said as text is kept. It is a snapshot of
// one link for a few seconds, never a record of the network, and it makes no finding.
type CaptureSummary struct {
	At time.Time `json:"at"`
	// Interface is the link listened on, and Wireless says whether it is Wi-Fi (a Wi-Fi link shows broadcast and multicast frames and the
	// traffic of this computer, nothing else).
	Interface string `json:"interface"`
	Wireless  bool   `json:"wireless"`
	// StoppedBy is why the window ended: window, frame_limit, cut, interface_missing, interface_down, socket_error or unsupported.
	StoppedBy string `json:"stopped_by"`
	ElapsedMs int    `json:"elapsed_ms"`
	// Frames is every frame heard, copies included. The typed counts below are of events: identical copies inside one burst (an access
	// point or mesh repeating a broadcast to a Wi-Fi client) are folded, and RepeatedCopies says how many were.
	Frames         int `json:"frames"`
	RepeatedCopies int `json:"repeated_copies"`
	MaxPerSec      int `json:"max_per_second"`
	KernelSeen     int `json:"kernel_seen"`
	// KernelDropped counts frames the kernel dropped because they came faster than they were read.
	KernelDropped int           `json:"kernel_dropped"`
	Counts        CaptureCounts `json:"counts"`
	// Claims are the hardware and protocol addresses ARP senders announced, at most 64, and DHCPServerMACs the Ethernet sources of DHCP
	// frames sent from port 67 to 68 (a DHCP server answering), at most 8. A second DHCP source is worth a look; it is not proof.
	Claims         []CaptureClaim `json:"claims"`
	DHCPServerMACs []string       `json:"dhcp_server_macs"`
	// ConflictingAddresses counts IPv4 addresses announced by more than one hardware address in this window.
	ConflictingAddresses int `json:"conflicting_addresses"`
	// OffNetworkAddresses counts announced IPv4 addresses outside every range this link has itself. It is how a device on another network
	// (a tagged VLAN whose broadcasts reach this link) shows: the kernel hands the capture the frame without its tag, so the address is the
	// only sign. It is a sign to look at, not proof, and 0 also covers a link whose own ranges are unknown.
	OffNetworkAddresses int `json:"off_network_addresses"`
}

// CaptureCounts is how many frames of each kind were seen.
type CaptureCounts struct {
	ARPRequests       int `json:"arp_requests"`
	ARPReplies        int `json:"arp_replies"`
	ARPGratuitous     int `json:"arp_gratuitous"`
	DHCPFromServer    int `json:"dhcp_from_server"`
	DHCPFromClient    int `json:"dhcp_from_client"`
	MDNS              int `json:"mdns"`
	NDRouterSolicit   int `json:"nd_router_solicit"`
	NDRouterAdvert    int `json:"nd_router_advert"`
	NDNeighborSolicit int `json:"nd_neighbor_solicit"`
	NDNeighborAdvert  int `json:"nd_neighbor_advert"`
	NDRedirect        int `json:"nd_redirect"`
	// Unreadable counts frames too short or of a shape the reader does not know; they are counted, never guessed.
	Unreadable int `json:"unreadable"`
}

// CaptureClaim is one address an ARP sender announced.
type CaptureClaim struct {
	MAC string `json:"mac"`
	IP  string `json:"ip"`
	// Gratuitous is true when the sender announced its own address (sender and target address the same).
	Gratuitous bool `json:"gratuitous"`
	// OffNetwork is true when the address is outside every range this link has itself.
	OffNetwork bool `json:"off_network"`
}
