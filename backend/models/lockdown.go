package models

import "time"

// Lockdown is the state of the lock down switch. While On, automations do not start, agents may not use the terminal or the
// network, and connectors' inbound messages are refused. No setting is changed, so lifting it returns everything as it was;
// the sessions and tokens ended when it was switched on stay ended.
type Lockdown struct {
	On    bool      `json:"on"`
	Since time.Time `json:"since,omitzero"`
	// SignedOut and TokensRevoked say what switching it on ended, for the screen.
	SignedOut     int `json:"signed_out,omitempty"`
	TokensRevoked int `json:"tokens_revoked,omitempty"`
}
