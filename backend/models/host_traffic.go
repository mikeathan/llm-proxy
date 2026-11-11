package models

import "time"

// HostTraffic is what one capture_host look saw of this computer's own traffic (plan network-host-traffic-and-findings-ui, Constitution I.13),
// read from the first bytes of each frame the helper's fixed filter kept. It is totals, ranked destinations and a flow list; no frame and no
// payload is kept. A name is text a peer chose (a DNS answer or, later, a TLS server name), so it appears only in Destinations and Flows, only
// on this view, and is never put in a finding, an alert, a log, a prompt or the Timeline. It is a snapshot of a few seconds, never a record of
// the network, and it makes no finding.
type HostTraffic struct {
	At        time.Time `json:"at"`
	Interface string    `json:"interface"`
	// StoppedBy is why the window ended: window, frame_limit, cut, interface_missing, interface_down, socket_error or unsupported.
	StoppedBy string `json:"stopped_by"`
	ElapsedMs int    `json:"elapsed_ms"`
	// Frames is how many frames were kept and read; Unreadable counts kept frames too short or of a shape the reader does not know.
	Frames     int `json:"frames"`
	Unreadable int `json:"unreadable"`
	// KernelSeen and KernelDropped are the packet socket's counters: frames that passed the filter, and frames the kernel dropped because
	// they came faster than they were read. Dropped frames are missing from every figure below.
	KernelSeen    int `json:"kernel_seen"`
	KernelDropped int `json:"kernel_dropped"`
	// Sample is the coarsest sampling rate in force during the window: 1 means every frame was counted, N means one ordinary frame in N was
	// kept and the byte figures are the kept bytes times N (an estimate). A DNS message is never sampled.
	Sample int `json:"sample"`
	// BytesIn and BytesOut are the bytes of IP packets to and from this computer; BytesThrough are packets that were neither (forwarded for
	// another machine). Internet and LocalNetwork split the same total by where the other end is.
	BytesIn      uint64 `json:"bytes_in"`
	BytesOut     uint64 `json:"bytes_out"`
	BytesThrough uint64 `json:"bytes_through"`
	// DirectionUnknown says no packet could be told to be this computer's: neither its addresses nor its network cards' hardware addresses
	// matched, so every byte is in BytesThrough. The screen says it could not tell which way traffic went, never that nothing was sent.
	DirectionUnknown bool `json:"direction_unknown,omitempty"`
	// SelfBytes is the part of the window that was this program's own web page or API being served (a browser viewing the page), which a look
	// taken from that page always contains; the screen sets it apart.
	SelfBytes    uint64 `json:"self_bytes,omitempty"`
	Internet     uint64 `json:"internet_bytes"`
	LocalNetwork uint64 `json:"local_network_bytes"`
	// Lookups is how many DNS questions this computer asked in the window.
	Lookups int `json:"lookups"`
	// Kinds is the bytes of each kind of traffic (a fixed word), biggest first. PerSecond is the bytes in each second of the window.
	Kinds     []TrafficKind `json:"kinds"`
	PerSecond []uint64      `json:"per_second"`
	// Destinations are the ten biggest places traffic went to or came from, by name when a DNS answer gave one, else by address.
	Destinations []TrafficDestination `json:"destinations"`
	// Flows are the biggest conversations, at most 24; OtherFlows and OtherBytes count the rest, and every flow past the table's 512.
	Flows      []TrafficFlow `json:"flows"`
	OtherFlows int           `json:"other_flows"`
	OtherBytes uint64        `json:"other_bytes"`
	// OwnersUsed says the table of address owners was downloaded when the window was read, so a missing owner means "not in the table" and not "no
	// table". Owners and Countries are the internet bytes of the table's conversations by network holder (at most 8) and by country (at most 12),
	// biggest first; they cover the conversations the table kept, so they can sum to less than Internet.
	OwnersUsed bool                  `json:"owners_used"`
	Owners     []TrafficOwnerShare   `json:"owners"`
	Countries  []TrafficCountryShare `json:"countries"`
}

// TrafficKind is the bytes of one kind of traffic. Kind is one of a fixed set of words (https, http, quic, dns, ssh, ntp, discovery, icmp,
// tcp, udp).
type TrafficKind struct {
	Kind  string `json:"kind"`
	Bytes uint64 `json:"bytes"`
}

// The two places a traffic view's name can come from. A name is a hint either way: a peer chose the text.
const (
	NameFromDNS = "dns" // the question of a DNS answer that gave the address
	NameFromTLS = "tls" // the server name in a TLS ClientHello this computer sent to the address
)

// TrafficDestination is the bytes exchanged with one place. Name is empty when no DNS answer in the window named the address.
type TrafficDestination struct {
	Name     string `json:"name,omitempty"`
	NameFrom string `json:"name_from,omitempty"`
	Address  string `json:"address"`
	Bytes    uint64 `json:"bytes"`
	Internet bool   `json:"internet"`
	// Shared marks a broadcast or multicast address many devices share (by its IP address or the frame's hardware address); never internet.
	Shared bool          `json:"shared,omitempty"`
	Owner  *TrafficOwner `json:"owner,omitempty"`
}

// TrafficFlow is one conversation with a remote address and port, by protocol. Port is the service's port: the remote's when this computer
// opened the conversation, this computer's own when Served (another machine connected to a service here). Self marks the conversations of this
// program's own port.
type TrafficFlow struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	NameFrom string `json:"name_from,omitempty"`
	Port     int    `json:"port"`
	Served   bool   `json:"served,omitempty"`
	Self     bool   `json:"self,omitempty"`
	Proto    string `json:"proto"`
	Kind     string `json:"kind"`
	Internet bool   `json:"internet"`
	// Shared marks a conversation with a broadcast or multicast address (see TrafficDestination.Shared).
	Shared   bool          `json:"shared,omitempty"`
	BytesOut uint64        `json:"bytes_out"`
	BytesIn  uint64        `json:"bytes_in"`
	Owner    *TrafficOwner `json:"owner,omitempty"`
}

// TrafficOwner is who runs the network an internet address belongs to, from a public table of address ranges kept on this computer (plan
// network-clickable-findings-traffic-owners, Constitution I.13). ASN is the autonomous system number, Country two capital letters, and Name the
// holder's own words in that table: text somebody else chose, so it appears only on this view and only as text, and is never put in a finding,
// an alert, a log, a prompt or the Timeline. Nothing is sent anywhere to look an address up.
type TrafficOwner struct {
	ASN     uint32 `json:"asn"`
	Name    string `json:"name"`
	Country string `json:"country,omitempty"`
	// Region and City are where the address is used, from a second public table (DB-IP Lite City) read from disk on this computer. They are
	// approximate (a content network shows the hub that serves an address, not the machine), empty until that table is downloaded, and text
	// somebody else chose, so they are shown only as text.
	Region string `json:"region,omitempty"`
	City   string `json:"city,omitempty"`
	// PlaceCountry is the country the city table puts the address in (two capital letters), which can differ from Country, the country the
	// range is registered to. Point is the city's latitude and longitude from the same table, for the map; approximate like the city.
	PlaceCountry string    `json:"place_country,omitempty"`
	Point        *GeoPoint `json:"point,omitempty"`
}

// GeoPoint is a latitude and a longitude in degrees, from the city table: where a city is, never where a machine stands.
type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// TrafficOwnerShare is the internet bytes that went to or came from one network holder.
type TrafficOwnerShare struct {
	TrafficOwner
	Bytes uint64 `json:"bytes"`
}

// TrafficCountryShare is the internet bytes that went to or came from addresses of one country (two capital letters).
type TrafficCountryShare struct {
	Country string `json:"country"`
	Bytes   uint64 `json:"bytes"`
}
