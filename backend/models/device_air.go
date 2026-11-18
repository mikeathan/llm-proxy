package models

import "time"

// AirState is whether a sensing stack (Radio) can say anything about the network's devices now (ADR-0061).
type AirState string

// The states of the air join.
const (
	// AirOff: no sensing stack is running, so the air says nothing.
	AirOff AirState = "off"
	// AirNotListening: the stack runs but has not finished a listening cycle in the last day, so "not heard" means nothing.
	AirNotListening AirState = "not_listening"
	// AirListening: the stack listened in the last day; a device not heard is "not heard while listening", still never "not there".
	AirListening AirState = "listening"
	// AirUnreadable: the stack's report could not be read; said as "could not read", never as a quiet air.
	AirUnreadable AirState = "unreadable"
)

// DeviceAir is what the air says of one inventory device, as fixed values (no name, no address, nothing a frame said). Heard is when it was last
// heard, zero when it was not heard lately; Band ("2G", "5G", "6G"), Ring ("room", "house", "nearby", "far") and Moving are known only while it is
// heard; Kind is what the sensing stack thinks it is; OnYourWiFi says it exchanged frames with one of the owner's confirmed access points.
type DeviceAir struct {
	Heard      time.Time `json:"heard,omitzero"`
	Band       string    `json:"band,omitempty"`
	Ring       string    `json:"ring,omitempty"`
	Moving     bool      `json:"moving,omitempty"`
	OnYourWiFi bool      `json:"on_your_wifi,omitempty"`
	Kind       string    `json:"kind,omitempty"`
}

// AirView is the join of the air with the network inventory at one moment: the state, the latest listening cycle, the bands listened on, and
// what the air says of each inventory device, by its hardware address (kept in the core: never sent to a stack, never shown).
type AirView struct {
	State     AirState             `json:"state"`
	LastCycle time.Time            `json:"last_cycle,omitzero"`
	Bands     []string             `json:"bands,omitempty"`
	ByMAC     map[string]DeviceAir `json:"-"`
}
