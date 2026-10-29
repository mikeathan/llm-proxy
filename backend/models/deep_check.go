package models

import "time"

// DeepCheck is the last deep check of one device (network-audit.md 13.J): when it ran, which ports it looked at and
// whether a TLS or an SSH service answered on each, and what it found. Keeping what answered is what lets a report say "no
// TLS service answered" instead of an empty list that reads as "nothing wrong".
type DeepCheck struct {
	Device   string           `json:"device"`
	At       time.Time        `json:"at"`
	Ports    []DeepPortResult `json:"ports"`
	Findings []ScanFinding    `json:"findings"`
}

// DeepPortResult is one port a deep check looked at. TLS is true when the TLS script produced output on it (a TLS service
// answered), SSH when the SSH script did; both false means nothing the deep check can judge answered there.
type DeepPortResult struct {
	Port int  `json:"port"`
	TLS  bool `json:"tls_answered"`
	SSH  bool `json:"ssh_answered"`
}
