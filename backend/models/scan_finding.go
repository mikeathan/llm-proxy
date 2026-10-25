package models

import (
	"fmt"
	"time"
)

// ScanFinding is one fact worth reporting about a host, identified by Key so a later scan can be compared with this
// one. Version and Evidence come from the network and are untrusted text; the scan engines clean them before they get
// here.
type ScanFinding struct {
	Device   string `json:"device"`
	Port     int    `json:"port,omitempty"`
	Proto    string `json:"proto,omitempty"`
	Service  string `json:"service,omitempty"`
	Version  string `json:"version,omitempty"`
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Evidence string `json:"evidence,omitempty"`
}

// Key identifies what a finding is about, independent of its wording, for comparing scans.
func (f ScanFinding) Key() string {
	return fmt.Sprintf("%s|%s|%d|%s", f.Device, f.Proto, f.Port, f.Check)
}

// ScanBaseline is the result of an automation's latest scan, kept to compare the next one against. Only the latest is
// kept.
type ScanBaseline struct {
	ScannedAt time.Time     `json:"scanned_at"`
	Engine    string        `json:"engine"`
	Findings  []ScanFinding `json:"findings"`
	// NewKeys are the keys of the findings that were not in the scan before this one, and Resolved the findings of that
	// earlier scan that are gone, so a screen can show what changed without keeping two scans.
	NewKeys  []string      `json:"new_keys,omitempty"`
	Resolved []ScanFinding `json:"resolved,omitempty"`
}

// ScanAck is the operator's decision to accept a finding for a while. It names the finding by Key and the version text
// that was accepted: if the finding's version changes, the acceptance no longer applies and the finding comes back.
type ScanAck struct {
	Key             string    `json:"key"`
	AcceptedVersion string    `json:"accepted_version,omitempty"`
	Note            string    `json:"note,omitempty"`
	AcceptedAt      time.Time `json:"accepted_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	// LastSeen is the last time a scan still showed the finding, so an acceptance for something long gone is dropped.
	LastSeen time.Time `json:"last_seen,omitempty"`
}
