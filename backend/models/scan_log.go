package models

import "time"

// ScanEntry is one audit scan as the window digest needs it: when it ran, whether the schedule or a person started it, and
// how it came out in numbers. The baseline keeps only the latest scan, so these rows are what lets a later look say how the
// network moved across a day. It holds counts and finding keys only (a key is device|proto|port|check), never a banner, a
// version or a note, and only the audit's report step writes it, never a run's model.
type ScanEntry struct {
	Automation string    `json:"automation"`
	At         time.Time `json:"at"`
	// Unattended is true when the schedule started the run, false when someone pressed Run.
	Unattended bool   `json:"unattended,omitempty"`
	Engine     string `json:"engine,omitempty"`
	DevicesUp  int    `json:"devices_up"`
	// OpenPorts is the open ports the scan showed; BySeverity counts the other findings by severity.
	OpenPorts  int            `json:"open_ports"`
	BySeverity map[string]int `json:"by_severity,omitempty"`
	// New and Resolved count what changed against the scan before; NewKeys and ResolvedKeys name the first few.
	New          int      `json:"new,omitempty"`
	Resolved     int      `json:"resolved,omitempty"`
	NewKeys      []string `json:"new_keys,omitempty"`
	ResolvedKeys []string `json:"resolved_keys,omitempty"`
}
