package models

import "time"

// UbuntuIndex is what the program keeps of Canonical's security notices for one Ubuntu release: for each binary package, the
// versions that fixed a published flaw and the notice that did it; and for each notice the flaws it covers. It holds ids,
// version strings and dates only, so nothing a notice's prose said can reach a finding.
type UbuntuIndex struct {
	// Release is the release's codename ("noble").
	Release string `json:"release,omitempty"`
	// GeneratedAt is when Canonical built the file, ImportedAt when this program read it.
	GeneratedAt time.Time               `json:"generated_at,omitzero"`
	ImportedAt  time.Time               `json:"imported_at,omitzero"`
	Notices     map[string]UbuntuNotice `json:"notices,omitempty"`
	Packages    map[string][]UbuntuFix  `json:"packages,omitempty"`
}

// UbuntuNotice is one security notice (USN): when it was issued (YYYY-MM-DD), the worst severity among its flaws, and their ids.
type UbuntuNotice struct {
	Issued   string   `json:"issued,omitempty"`
	Severity string   `json:"severity,omitempty"`
	CVEs     []string `json:"cves,omitempty"`
}

// UbuntuFix says that a package at a version older than Fixed is missing the fix in notice USN.
type UbuntuFix struct {
	Fixed string `json:"fixed"`
	USN   string `json:"usn"`
}
