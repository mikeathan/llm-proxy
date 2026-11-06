package models

import "time"

// NetworkAlertKind says which Network alert an entry in the alert log is.
type NetworkAlertKind string

const (
	NetworkAlertDecoy    NetworkAlertKind = "decoy"
	NetworkAlertPresence NetworkAlertKind = "presence"
	NetworkAlertSummary  NetworkAlertKind = "summary"
	// NetworkAlertMonitor is a part of the module that is switched on and stopped working (monitor_stalled).
	NetworkAlertMonitor NetworkAlertKind = "monitor"
	// NetworkAlertSignIn is an admin sign-in event: an address locked out, or a sign-in from a new address.
	NetworkAlertSignIn NetworkAlertKind = "sign_in"
	// NetworkAlertFollowUp is the result of the scan on arrival of one new device. It is its own kind so the Timeline does not
	// fold it into the new-device row it follows.
	NetworkAlertFollowUp NetworkAlertKind = "follow_up"
	// NetworkAlertAgent is from the agent security record: an unattended run refused by the safety rules, or a stable automation
	// fetching from a new host.
	NetworkAlertAgent NetworkAlertKind = "agent"
	// NetworkAlertPhoneHome is from the phone-home watch: a device asked for a name it had not asked for in its first week, or asks
	// for a new name at a steady beat.
	NetworkAlertPhoneHome NetworkAlertKind = "phone_home"
	// NetworkAlertRouter is from the router's log: a burst of failed sign-ins to its web page, or of Wi-Fi disconnects.
	NetworkAlertRouter NetworkAlertKind = "router"
	// NetworkAlertLogin is from the record of failed logins to this computer: a burst of them.
	NetworkAlertLogin NetworkAlertKind = "login"
)

// Valid reports whether k is one of the known kinds.
func (k NetworkAlertKind) Valid() bool {
	switch k {
	case NetworkAlertDecoy, NetworkAlertPresence, NetworkAlertSummary, NetworkAlertMonitor, NetworkAlertSignIn, NetworkAlertFollowUp,
		NetworkAlertAgent, NetworkAlertPhoneHome, NetworkAlertRouter, NetworkAlertLogin:
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
