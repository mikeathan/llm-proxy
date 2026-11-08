package dnscheck

import "time"

// Outcome is how one comparison of the router's DNS answer and an outside resolver's ended.
type Outcome string

const (
	// Match means both answered and gave the same addresses.
	Match Outcome = "match"
	// Differ means both answered and the addresses differ.
	Differ Outcome = "differ"
	// CannotCompare means at least one side gave no usable answer. It is never read as a match.
	CannotCompare Outcome = "cannot_compare"
)

// Reasons a comparison could not be made. They are fixed words: nothing a resolver says is kept.
const (
	ReasonNoRouter     = "no_router"
	ReasonRouterSilent = "router_no_answer"
	ReasonOutsideFails = "outside_failed"
)

// Comparison is one name asked of the router's DNS and of an outside resolver. Name is one the operator chose (checked to be
// a host name); the addresses are parsed addresses, never answer text. The JSON is the log's file format and has not changed.
type Comparison struct {
	At      time.Time `json:"at"`
	Name    string    `json:"name"`
	Outcome Outcome   `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
	// Overlap is true when the two answers shared at least one address.
	Overlap bool     `json:"overlap,omitempty"`
	Router  []string `json:"router,omitempty"`
	Outside []string `json:"outside,omitempty"`
}
