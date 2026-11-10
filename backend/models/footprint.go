package models

import "time"

// How often a footprint watch is looked at on its own. A watch can always be looked at by hand ("Check now").
const (
	FootprintManual = "manual"
	FootprintDaily  = "daily"
	FootprintWeekly = "weekly"
)

// DomainFindingPrefix and EmailFindingPrefix start the Finding.Device of a footprint finding: a finding about a domain or an
// address has no device, so, like the computer's own findings, it carries a fixed label instead of an IP address. The email label
// holds an opaque id assigned when the address was added, never the address or anything derived from it.
const (
	DomainFindingPrefix = "domain:"
	EmailFindingPrefix  = "email:"
)

// FootprintDomain is a domain to watch.
type FootprintDomain struct {
	Name      string `yaml:"name" json:"name"`
	Frequency string `yaml:"frequency,omitempty" json:"frequency"`
}

// FootprintEmail is an address to watch. ID is assigned when it is added and never changes; findings and results are keyed on it.
type FootprintEmail struct {
	ID        string `yaml:"id" json:"id"`
	Address   string `yaml:"address" json:"address"`
	Frequency string `yaml:"frequency,omitempty" json:"frequency"`
}

// FootprintConfig is what the operator watches outside the local network and which outside services may be asked. Each service
// switch is off until the operator turns it on, and each says what that service learns (see the Footprint page).
type FootprintConfig struct {
	Domains []FootprintDomain `yaml:"domains,omitempty" json:"domains"`
	Emails  []FootprintEmail  `yaml:"emails,omitempty" json:"emails"`
	// CertificatesOn asks a public certificate log which certificates exist for a domain (the log learns the domain).
	CertificatesOn bool `yaml:"certificates_on,omitempty" json:"certificates_on"`
	// ExpiryOn asks the domain's registry when it expires (the registry's server learns the domain).
	ExpiryOn bool `yaml:"expiry_on,omitempty" json:"expiry_on"`
	// MailRecordsOn asks a DNS-over-HTTPS resolver for a domain's SPF and DMARC records (the resolver learns the domain).
	MailRecordsOn bool `yaml:"mail_records_on,omitempty" json:"mail_records_on"`
	// BreachesOn asks a breach service about an address with the k-anonymity search: only the first six characters of the
	// address's SHA-1 hash leave this computer. It needs the operator's own key, kept in the secrets store.
	BreachesOn bool `yaml:"breaches_on,omitempty" json:"breaches_on"`
}

// Footprint check ids: what one look at one watch produced. A result holds one per (watch, check).
const (
	FootprintCheckCerts   = "certificates"
	FootprintCheckExpiry  = "expiry"
	FootprintCheckMail    = "mail_records"
	FootprintCheckBreach  = "breaches"
	FootprintLooked       = "looked"
	FootprintCouldNotLook = "could_not_look"
)

// FootprintResult is the latest answer of one check on one watch. Status is FootprintLooked or FootprintCouldNotLook; a look that
// could not be made carries a fixed Reason code and never reads as "nothing found". The typed facts are numbers, flags and
// validated short names only, never text an outside service chose.
type FootprintResult struct {
	Watch  string    `json:"watch"` // the finding label: "domain:example.com" or "email:<id>"
	Check  string    `json:"check"`
	Status string    `json:"status"`
	Reason string    `json:"reason,omitempty"`
	// At is the latest attempt; LookedAt is the last look that worked. After a failed attempt the typed facts of the last good look
	// are kept, so a service that is down never erases what was known.
	At       time.Time `json:"at"`
	LookedAt time.Time `json:"looked_at,omitzero"`

	// Certificates: the ids seen so far (bounded), how many are new since the last look, and how many names the log holds.
	CertIDs  []int64 `json:"cert_ids,omitempty"`
	NewCerts int     `json:"new_certs,omitempty"`
	// Expiry: the registry's expiration date.
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	// Mail records.
	SPF   string `json:"spf,omitempty"`   // "missing", "weak" or "ok"
	DMARC string `json:"dmarc,omitempty"` // "missing", "none" or "ok"
	// Breaches: how many breaches the address was in, and their validated names and dates (as the service gave them).
	Breaches []FootprintBreach `json:"breaches,omitempty"`
}

// FootprintBreach is one breach an address was found in: a validated short name only.
type FootprintBreach struct {
	Name string `json:"name"`
}
