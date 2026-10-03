// notify.go — per-automation result delivery: the NotifyConfig an operator sets
// on an automation, and the SeenLedger that lets recurring digests skip items
// they already reported.
package models

import (
	"errors"
	"fmt"
	"time"
)

// DefaultDedupDays is how long a reported item stays in the seen ledger when
// NotifyConfig.DedupDays is unset. It matches the 60-day recency window the
// release-brief template asks the agent to search within.
const DefaultDedupDays = 60

// NotifyConfig delivers an automation's final report through a configured
// communication connector once the run succeeds. Delivery is performed by the
// dispatcher, not by the agent, so it works under any network grant and never
// depends on the model calling a tool.
type NotifyConfig struct {
	// Connector is the name of a configured communication connector (the key in
	// registry.json communication.connectors, e.g. "my-telegram").
	Connector string `yaml:"connector" json:"connector"`
	// Dedup drops table rows whose link was already reported in the last
	// DedupDays days, and tells the agent which titles to skip.
	Dedup bool `yaml:"dedup,omitempty" json:"dedup,omitempty"`
	// DedupDays is the ledger retention; <= 0 means DefaultDedupDays.
	DedupDays int `yaml:"dedup_days,omitempty" json:"dedup_days,omitempty"`
	// SendEmpty sends a one-line "nothing new" message when dedup removed every
	// row; false (default) stays silent.
	SendEmpty bool `yaml:"send_empty,omitempty" json:"send_empty,omitempty"`
}

// Validate rejects a delivery block that cannot work: it needs a connector
// name, and a negative retention is meaningless (0 = default).
func (n NotifyConfig) Validate() error {
	if n.Connector == "" {
		return errors.New("connector is required")
	}
	if n.DedupDays < 0 {
		return fmt.Errorf("dedup_days must not be negative, got %d", n.DedupDays)
	}
	return nil
}

// RetentionDays resolves DedupDays against the default.
func (n NotifyConfig) RetentionDays() int {
	if n.DedupDays <= 0 {
		return DefaultDedupDays
	}
	return n.DedupDays
}

// SeenEntry records one reported item.
type SeenEntry struct {
	Title string    `json:"title"`
	At    time.Time `json:"at"`
}

// SeenLedger maps a canonical item URL to when it was last reported.
type SeenLedger map[string]SeenEntry
