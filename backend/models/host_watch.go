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
	// Flaws is the last judgement of the installed packages against Ubuntu's security notices, kept for the same reason.
	Flaws HostFlaws `json:"flaws,omitzero"`
	// Privilege is the first look at the accounts, the admin groups, the kernel modules and the set-id programs, which later looks
	// are compared with.
	Privilege HostPrivilege `json:"privilege,omitzero"`
}

// HostPrivilege is what the first look at this computer's privilege marks found. Each list is learned once, the first time it
// could be read in full, and never updated: an addition stays a finding until the operator accepts it, like a new open port.
// The names are plain tokens and the paths plain absolute paths, which the platform reader has already checked.
type HostPrivilege struct {
	AccountsSeen  bool     `json:"accounts_seen,omitempty"`
	LoginAccounts []string `json:"login_accounts,omitempty"`
	Admins        []string `json:"admins,omitempty"`
	ModulesSeen   bool     `json:"modules_seen,omitempty"`
	Modules       []string `json:"modules,omitempty"`
	SetIDSeen     bool     `json:"set_id_seen,omitempty"`
	SetID         []string `json:"set_id,omitempty"`
}

// HostFlaws is what the last judgement of the installed packages found: how many are older than the version that fixed a
// published flaw, up to ten of their names, how many flaws that is and how many of those CISA lists as exploited, and the worst
// severity among them.
type HostFlaws struct {
	Packages  int      `json:"packages,omitempty"`
	Names     []string `json:"names,omitempty"`
	Flaws     int      `json:"flaws,omitempty"`
	Exploited int      `json:"exploited,omitempty"`
	Severity  string   `json:"severity,omitempty"`
}

// HostIntegrity is the outcome of the last complete pass over the packaged program files: how many differed and up to ten of
// their paths, which the platform reader has already limited to plain absolute paths.
type HostIntegrity struct {
	Modified int       `json:"modified,omitempty"`
	Paths    []string  `json:"paths,omitempty"`
	At       time.Time `json:"at,omitzero"`
}
