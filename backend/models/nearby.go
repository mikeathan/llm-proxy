package models

import "time"

// NearbyDevice is what one device on the network said about itself when asked over mDNS and SSDP (Constitution I.9). Every
// text field came from the device: it is cleaned and cut before it gets here, shown on screen as plain text only, and never
// put in a finding, an alert, a log line or a prompt.
type NearbyDevice struct {
	IP string `json:"ip"`
	// Names are the service instance names it announced (at most three).
	Names []string `json:"names,omitempty"`
	// Services are service types such as "_ipp._tcp" (at most sixteen, each matching a fixed pattern).
	Services []string `json:"services,omitempty"`
	// Ports are the ports its service records named.
	Ports []int `json:"ports,omitempty"`
	// UPnPType is the end of its UPnP device type URN, such as "InternetGatewayDevice:1".
	UPnPType string `json:"upnp_type,omitempty"`
	Maker    string `json:"maker,omitempty"`
	Model    string `json:"model,omitempty"`
	// Gateway is true while its UPnP description says it is an Internet Gateway Device.
	Gateway bool `json:"gateway,omitempty"`
	// Sources are "mdns" and "ssdp": how it was heard.
	Sources    []string  `json:"sources,omitempty"`
	FirstHeard time.Time `json:"first_heard"`
	LastHeard  time.Time `json:"last_heard"`
}

// NearbyState is the result of the latest looks, kept in one file.
type NearbyState struct {
	Devices []NearbyDevice `json:"devices"`
	// LastLook is when the latest look ended, and LastOutcome how: "heard", "nothing_heard", or a fixed reason it could not run.
	LastLook    time.Time `json:"last_look,omitzero"`
	LastOutcome string    `json:"last_outcome,omitempty"`
}
