package models

import "time"

// HostListener is a port that was open to other computers when the host watch first looked.
type HostListener struct {
	Proto string `json:"proto"`
	Port  int    `json:"port"`
}

// HostFile is the digest the host watch first recorded for one watched place (see hostwatch.FileSpec). Tag identifies the
// run of the program for the program's own file; its digest is compared only within one run.
type HostFile struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
	Tag  string `json:"tag,omitempty"`
}

// HostWatch is what the host watch compared against: the open ports and the file digests of its first look. A zero
// ListenersAt means no baseline was taken yet, which is different from a baseline with no ports.
type HostWatch struct {
	ListenersAt time.Time      `json:"listeners_at,omitzero"`
	Listeners   []HostListener `json:"listeners,omitempty"`
	Files       []HostFile     `json:"files,omitempty"`
	// Integrity is the last complete comparison of the program files with their packages' checksums, kept so a later pass
	// that is cut short does not resolve a finding the earlier one raised.
	Integrity HostIntegrity `json:"integrity,omitzero"`
}

// HostIntegrity is the outcome of the last complete pass over the packaged program files: how many differed and up to ten of
// their paths, which the platform reader has already limited to plain absolute paths.
type HostIntegrity struct {
	Modified int       `json:"modified,omitempty"`
	Paths    []string  `json:"paths,omitempty"`
	At       time.Time `json:"at,omitzero"`
}
