package models

import "time"

// NameSuggestions are what devices called themselves when asked, kept so the Devices card can offer a name to confirm.
// A name is untrusted text a device sent: it is cleaned and bounded before it is stored, shown on screen only, and never
// put in a finding, an alert or a prompt.
type NameSuggestions struct {
	Entries []NameSuggestion `json:"entries"`
}

// NameSuggestion is the last question put to one device and, if it answered, what it said.
type NameSuggestion struct {
	MAC string `json:"mac"`
	// Name is the cleaned answer; empty while the device has never answered.
	Name string `json:"name,omitempty"`
	// TriedAt is when the device was last asked, AnsweredAt when it last gave a name.
	TriedAt    time.Time `json:"tried_at"`
	AnsweredAt time.Time `json:"answered_at,omitzero"`
	// Misses counts the questions in a row that got no usable answer; it sets how long to wait before asking again.
	Misses int `json:"misses,omitempty"`
}
