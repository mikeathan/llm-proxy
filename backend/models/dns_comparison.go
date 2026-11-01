package models

import "time"

// DNSOutcome is how one comparison of the router's DNS answer and an outside resolver's ended.
type DNSOutcome string

const (
	// DNSMatch means both answered and gave the same addresses.
	DNSMatch DNSOutcome = "match"
	// DNSDiffer means both answered and the addresses differ.
	DNSDiffer DNSOutcome = "differ"
	// DNSCannotCompare means at least one side gave no usable answer. It is never read as a match.
	DNSCannotCompare DNSOutcome = "cannot_compare"
)

// DNS reasons a comparison could not be made. They are fixed words: nothing a resolver says is kept.
const (
	DNSReasonNoRouter     = "no_router"
	DNSReasonRouterSilent = "router_no_answer"
	DNSReasonOutsideFails = "outside_failed"
)

// DNSComparison is one name asked of the router's DNS and of an outside resolver. Name is one the operator chose (checked to be
// a host name); the addresses are parsed addresses, never answer text.
type DNSComparison struct {
	At      time.Time  `json:"at"`
	Name    string     `json:"name"`
	Outcome DNSOutcome `json:"outcome"`
	Reason  string     `json:"reason,omitempty"`
	// Overlap is true when the two answers shared at least one address.
	Overlap bool     `json:"overlap,omitempty"`
	Router  []string `json:"router,omitempty"`
	Outside []string `json:"outside,omitempty"`
}
