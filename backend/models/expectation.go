package models

import "time"

// Expectation is what the operator says a device should offer: the TCP ports that may be open on it. Anything else open
// is reported as unexpected, and a listed port that does not answer as closed. An empty Ports means nothing should
// answer. The device is its IP address, because the scan has no better identity (no MAC without raw sockets).
type Expectation struct {
	Device string    `json:"device"`
	Ports  []int     `json:"ports"`
	Note   string    `json:"note,omitempty"`
	SetAt  time.Time `json:"set_at"`
}
