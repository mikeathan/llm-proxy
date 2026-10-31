package models

import "time"

// DevicePorts is the ports each device had open at its last completed scan. It holds port numbers and protocols only,
// never a banner, product or version, so nothing a device printed can reach it.
type DevicePorts struct {
	Devices []DevicePortRecord `json:"devices"`
}

// DevicePortRecord is one device's open ports, keyed by its hardware address so the record follows the device when its IP
// address changes.
type DevicePortRecord struct {
	MAC       string    `json:"mac"`
	Ports     []PortRef `json:"ports"`
	ScannedAt time.Time `json:"scanned_at"`
}

// PortRef is an open port: its number and protocol.
type PortRef struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"`
}
