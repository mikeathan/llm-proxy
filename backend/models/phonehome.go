package models

import "time"

// PhoneHome is what the phone-home watch remembers of the DNS names each device asks for: per device (keyed by hardware address,
// never by IP address) the names, when each was first and last asked for and, for a name that is new, a few recent times to see
// whether it is asked for at a steady beat. No query line is kept, only these facts, and every list has a fixed bound.
type PhoneHome struct {
	// Since is when the watch first recorded a lookup; "no lookups for a day" is judged only after it has watched for a day.
	Since   time.Time         `json:"since,omitzero"`
	Devices []PhoneHomeDevice `json:"devices"`
}

// PhoneHomeDevice is one device's names.
type PhoneHomeDevice struct {
	MAC string `json:"mac"`
	// First is when the watch first saw a lookup from the device; its names are learned for a week from then.
	First time.Time `json:"first"`
	// Last is the latest lookup.
	Last  time.Time       `json:"last"`
	Names []PhoneHomeName `json:"names"`
}

// PhoneHomeName is one name a device asked for. Times holds up to a fixed number of recent lookups, kept only while the name is new.
type PhoneHomeName struct {
	Name  string      `json:"name"`
	First time.Time   `json:"first"`
	Last  time.Time   `json:"last"`
	Count int         `json:"count"`
	Times []time.Time `json:"times,omitempty"`
}
