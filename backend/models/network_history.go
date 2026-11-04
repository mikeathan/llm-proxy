package models

import "time"

// NetworkHistory is the small, bounded record the Network page draws its trends from: one entry per day, never a list of
// probes. It holds counts only, so nothing a device printed can reach it.
type NetworkHistory struct {
	Days []NetworkDay `json:"days"`
	// DigestAt is when the last weekly summary was sent; zero before the first.
	DigestAt time.Time `json:"digest_at,omitzero"`
}

// NetworkDay is what one calendar day (in the host's local time) looked like.
type NetworkDay struct {
	// Date is the day as YYYY-MM-DD.
	Date string `json:"date"`
	// DevicesUp is the most devices seen answering at one look that day.
	DevicesUp int `json:"devices_up"`
	// Devices is how many devices the inventory knew at the day's last look, and Unnamed how many of those have no name.
	Devices int `json:"devices"`
	Unnamed int `json:"unnamed"`
	// Findings is the number of open findings at the day's last audit, and Serious the high and critical ones among them.
	Findings int `json:"findings"`
	Serious  int `json:"serious"`
	// Audited is true once an audit ran that day, so a day without one is not drawn as "no findings".
	Audited bool `json:"audited,omitempty"`
	// PortsOpened and PortsClosed are the ports that opened or closed since each device's previous scan, added up over the
	// day's audits.
	PortsOpened int `json:"ports_opened,omitempty"`
	PortsClosed int `json:"ports_closed,omitempty"`
	// Blocked is how many connection attempts from the internet the router refused that day, as its own log said, and
	// BlockedPorts the ports most often tried (at most ten). Counts only: no address and no line of the log is kept. The log
	// is unauthenticated UDP, so these are hints.
	Blocked      int         `json:"blocked,omitempty"`
	BlockedPorts []PortCount `json:"blocked_ports,omitempty"`
}

// PortCount is how many times a destination port was tried.
type PortCount struct {
	Port  int `json:"port"`
	Count int `json:"count"`
}
