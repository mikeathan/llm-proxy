package models

import "time"

// NetworkAlertKind says which Network alert an entry in the alert log is.
type NetworkAlertKind string

const (
	NetworkAlertDecoy    NetworkAlertKind = "decoy"
	NetworkAlertPresence NetworkAlertKind = "presence"
	NetworkAlertSummary  NetworkAlertKind = "summary"
)

// Valid reports whether k is one of the known kinds.
func (k NetworkAlertKind) Valid() bool {
	switch k {
	case NetworkAlertDecoy, NetworkAlertPresence, NetworkAlertSummary:
		return true
	}
	return false
}

// NetworkAlertDelivery says what became of the send to the operator's connectors.
type NetworkAlertDelivery string

const (
	// NetworkAlertSending is the moment between writing the entry and the send returning.
	NetworkAlertSending     NetworkAlertDelivery = "sending"
	NetworkAlertSent        NetworkAlertDelivery = "sent"
	NetworkAlertNoConnector NetworkAlertDelivery = "no_connector"
	NetworkAlertFailed      NetworkAlertDelivery = "failed"
)

// NetworkAlert is one entry of the Network alert log: the exact text sent to the connectors, and what became of the send.
// The text is fixed wording (no device name); only the failure's word is kept, never its reason.
type NetworkAlert struct {
	// ID rises strictly, so a client's cursor never skips an entry; an entry that is updated takes a new ID.
	ID       int64                `json:"id"`
	Kind     NetworkAlertKind     `json:"kind"`
	Message  string               `json:"message"`
	At       time.Time            `json:"at"`
	Delivery NetworkAlertDelivery `json:"delivery"`
	// Count is how many times a repeating decoy alert was raised for the same sources; 1 for everything else.
	Count int `json:"count"`
}
