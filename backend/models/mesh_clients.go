package models

import "time"

// MeshClient is one device joined to one box of the owner's mesh router, as the router's list of joined devices says (Constitution I.15, ADR-0059).
// It is kept in memory only. Its address on this network is dropped when the list is read.
type MeshClient struct {
	// MAC is the device's hardware address, lower case; Box is the hardware address of the mesh box it is joined to.
	MAC string
	Box string
	// Band is "2G", "5G" or "6G"; Guest says it is the guest network on that band.
	Band  string
	Guest bool
	// RSSI is the box's signal of the device, in dBm.
	RSSI int
}

// MeshClients is one read of the list, with when it was read.
type MeshClients struct {
	At      time.Time
	Clients []MeshClient
}
