package models

import "time"

// DecoyTouch records that one device connected to one decoy service: who, which decoy, when, and how many times. No byte
// the device sent is kept.
type DecoyTouch struct {
	Source string    `json:"source"`
	Port   int       `json:"port"`
	First  time.Time `json:"first"`
	Last   time.Time `json:"last"`
	Count  int       `json:"count"`
	// Notified means the operator was told about this source and port.
	Notified bool `json:"notified,omitempty"`
}
