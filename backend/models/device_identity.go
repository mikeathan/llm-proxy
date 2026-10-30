package models

import "time"

// DeviceIdentity is what one service on one device presented the last time it was looked at: the fingerprints of its SSH
// host keys, or of its TLS certificate. Only fingerprints and a short hash are kept, never the text a device sent.
type DeviceIdentity struct {
	Device string `json:"device"`
	Port   int    `json:"port"`
	// Kind is "ssh_hostkey" or "tls_cert".
	Kind string `json:"kind"`
	// Prints are the sorted fingerprints.
	Prints []string `json:"prints"`
	// Shape is, for a certificate, a hash of its subject, issuer and key type and size, so a renewal (the same shape, a new
	// fingerprint) can be told from a certificate for something else. Empty for SSH.
	Shape string    `json:"shape,omitempty"`
	Seen  time.Time `json:"seen"`
	// Change is set when the identity differs from the one before it, and stays until it ages out.
	Change *IdentityChange `json:"change,omitempty"`
}

// IdentityChange records that a service's identity was replaced.
type IdentityChange struct {
	At time.Time `json:"at"`
	// Renewal is true for a certificate whose subject, issuer and key type and size are unchanged.
	Renewal bool `json:"renewal,omitempty"`
}
