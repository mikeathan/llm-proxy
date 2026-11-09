package hosttraffic

// The fixed words for the kind of traffic a flow is, from its protocol and the remote port. The screen words them; nothing a peer sent
// chooses one.
const (
	kindHTTPS     = "https"
	kindHTTP      = "http"
	kindQUIC      = "quic"
	kindDNS       = "dns"
	kindSSH       = "ssh"
	kindNTP       = "ntp"
	kindDiscovery = "discovery"
	kindICMP      = "icmp"
	kindTCP       = "tcp"
	kindUDP       = "udp"

	protoNameTCP  = "tcp"
	protoNameUDP  = "udp"
	protoNameICMP = "icmp"
)

// kindOf says what kind of traffic one conversation is: by protocol and the remote port.
func kindOf(proto byte, rport uint16) string {
	switch proto {
	case protoICMP, protoICMPv6:
		return kindICMP
	case protoTCP:
		return tcpKinds[rport] + otherKind(tcpKinds[rport], kindTCP)
	case protoUDP:
		return udpKinds[rport] + otherKind(udpKinds[rport], kindUDP)
	}
	return kindUDP
}

func otherKind(found, fallback string) string {
	if found != "" {
		return ""
	}
	return fallback
}

var tcpKinds = map[uint16]string{443: kindHTTPS, 80: kindHTTP, 8080: kindHTTP, 22: kindSSH, portDNS: kindDNS}

var udpKinds = map[uint16]string{443: kindQUIC, portDNS: kindDNS, 123: kindNTP, 5353: kindDiscovery, 1900: kindDiscovery}

// protoName is the word for a flow's protocol.
func protoName(proto byte) string {
	switch proto {
	case protoTCP:
		return protoNameTCP
	case protoUDP:
		return protoNameUDP
	}
	return protoNameICMP
}
