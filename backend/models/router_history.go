package models

import "time"

// RouterHistory is what the router watch remembers of what each device on the network connects to (Constitution I.15): per device (keyed by
// hardware address, else by the address on this network) the companies and countries its connections went to and when each was first and last
// seen. No address, port or connection is kept, only these facts, and every list has a fixed bound.
type RouterHistory struct {
	Devices map[string]RouterHistoryDevice `json:"devices"`
}

// RouterHistoryDevice is one device's history.
type RouterHistoryDevice struct {
	// IP is the address the device had at its last read, so a finding can name it.
	IP        string                       `json:"ip"`
	First     time.Time                    `json:"first"`
	Last      time.Time                    `json:"last"`
	Companies map[string]RouterHistorySeen `json:"companies,omitempty"`
	Countries map[string]RouterHistorySeen `json:"countries,omitempty"`
}

// RouterHistorySeen is one company (keyed by its AS number) or country (keyed by its two letters) a device connected to. Name is the holder's
// own words from a public table: text only.
type RouterHistorySeen struct {
	Name  string    `json:"name"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}
