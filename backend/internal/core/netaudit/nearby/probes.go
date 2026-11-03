package nearby

import "net/netip"

// The two datagrams of Constitution I.9, built in code and never shaped by an operator, a model or a device.
var (
	// MDNSGroup and SSDPGroup are where they go.
	MDNSGroup = netip.MustParseAddrPort("224.0.0.251:5353")
	SSDPGroup = netip.MustParseAddrPort("239.255.255.250:1900")
)

// MDNSQuestion is the question "which service types does anyone offer": a PTR for _services._dns-sd._udp.local, class IN, with
// the unicast-response bit left clear. It is sent from an ephemeral port, so a responder answers by unicast (RFC 6762 §6.7).
var MDNSQuestion = []byte{
	0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, // id 0, flags 0, one question
	9, '_', 's', 'e', 'r', 'v', 'i', 'c', 'e', 's',
	7, '_', 'd', 'n', 's', '-', 's', 'd',
	4, '_', 'u', 'd', 'p',
	5, 'l', 'o', 'c', 'a', 'l',
	0,
	0, 12, 0, 1, // PTR, IN
}

// SSDPSearch asks every UPnP root device to describe where its page is, answering within two seconds.
var SSDPSearch = []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: upnp:rootdevice\r\n\r\n")
