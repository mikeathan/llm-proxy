package models

import "time"

// PhoneHome is what the phone-home watch remembers of the DNS names each device asks for: per device (keyed by hardware address,
// never by IP address) the names, when each was first and last asked for and, for a name that is new, a few recent times to see
// whether it is asked for at a steady beat. No query line is kept, only these facts, and every list has a fixed bound.
type PhoneHome struct {
	// Since is when the watch first recorded a lookup; "no lookups for a day" is judged only after it has watched for a day.
	Since   time.Time         `json:"since,omitzero"`
	Devices []PhoneHomeDevice `json:"devices"`
	// Told remembers which new places and beats the operator was already told of (device, kind and a digest of the names), so a
	// restart does not tell them again. It is bounded; the oldest entries go first.
	Told []PhoneHomeTold `json:"told,omitempty"`
}

// PhoneHomeTold is one alert that was sent.
type PhoneHomeTold struct {
	Key string    `json:"key"`
	At  time.Time `json:"at"`
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

// KnownBad is the list of host names abuse.ch's URLhaus says serve malware, as last downloaded. It is public data (the operator's
// address for it, which holds their key, is not kept here but in the secrets store).
type KnownBad struct {
	LoadedAt time.Time `json:"loaded_at,omitzero"`
	Names    []string  `json:"names"`
}

// DeviceLookups is what one device asked the router's DNS for, as the screen shows it: counts, the flags the watch raised, and the newest names.
// It is lookups, not traffic: the router's log line has no address and no bytes. Names are text a device chose and are shown as text only.
type DeviceLookups struct {
	MAC   string    `json:"mac"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
	// Today is how many different names it asked for in the last 24 hours; Total how many the watch keeps for it.
	Today int `json:"today"`
	Total int `json:"total"`
	// Flags are fixed words (new_place, beaconing, known_bad, encrypted_dns, around_dns); a flag that names names says which in Names.
	Flags []string           `json:"flags"`
	Names []DeviceLookupName `json:"names"`
}

// DeviceLookupName is one name a device asked for. Domain is the part a person recognises (googleapis.com for a.b.googleapis.com), found without
// any lookup; Flag is empty or the one fixed word that makes this name worth a look.
type DeviceLookupName struct {
	Name   string    `json:"name"`
	Domain string    `json:"domain"`
	Last   time.Time `json:"last"`
	Count  int       `json:"count"`
	Flag   string    `json:"flag,omitempty"`
}
