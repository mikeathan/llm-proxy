// Package framewire reads the first 64 bytes of the frames a capture_window run kept into typed counts (Constitution I.11: untrusted data
// is parsed in the unprivileged service, never in the privileged helper). It reads fixed offsets, uses the IPv4 header length for the
// UDP ports, counts a frame that is too short or of an unknown shape as unreadable, and keeps no text a device sent: only numbers and the
// hardware and protocol addresses of ARP senders and DHCP sources.
package framewire

import (
	"encoding/binary"
	"net/netip"
	"slices"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/models"
)

const (
	maxClaims      = 64
	maxDHCPServers = 8
	msPerSecond    = 1000

	etherTypeARP  = 0x0806
	etherTypeIPv4 = 0x0800
	etherTypeIPv6 = 0x86dd
	ethHeader     = 14
	arpLen        = 42 // Ethernet header and the 28-byte ARP body for Ethernet and IPv4
	ipv4Min       = 20
	udpHeader     = 4 // source and destination port: all that is read
	ipv6HeaderEnd = 54
	icmpv6Proto   = 58
	udpProto      = 17
	portDHCPSrv   = 67
	portDHCPCli   = 68
	portMDNS      = 5353
	fragmentMask  = 0x1fff
	// copyWindowMs is how close in time two identical frames must be to be copies of one burst, not two events.
	copyWindowMs = 100
)

// StoppedBy words, one per reason the helper or the decoder reports.
var stopWords = map[wire.CaptureStop]string{
	wire.StopWindow: "window", wire.StopFrameCap: "frame_limit", wire.StopNoInterface: "interface_missing", wire.StopInterfaceDown: "interface_down",
	wire.StopSocketError: "socket_error", wire.StopUnsupported: "unsupported", wire.StopCut: "cut",
}

// Summarize decodes one run's output and reads each frame. The interface, the time and whether the link is Wi-Fi are the caller's to fill in:
// they come from the request and the host, not from the frames.
func Summarize(data []byte) (models.CaptureSummary, error) {
	records, trailer, err := wire.DecodeCapture(data)
	if err != nil {
		return models.CaptureSummary{}, err
	}
	s := models.CaptureSummary{
		StoppedBy: stopWords[trailer.Stop], ElapsedMs: int(trailer.ElapsedMs), Frames: len(records),
		KernelSeen: int(trailer.KernelSeen), KernelDropped: int(trailer.KernelDropped),
		Claims: []models.CaptureClaim{}, DHCPServerMACs: []string{},
	}
	perSecond := map[uint32]int{}
	claimed := map[string]map[string]bool{} // ip -> hardware addresses that announced it
	seenClaim := map[[2]string]bool{}
	lastCopy := map[string]uint32{} // frame bytes -> when the last identical copy arrived
	for _, r := range records {
		perSecond[r.OffsetMs/msPerSecond]++
		key := string(r.Frame)
		if at, ok := lastCopy[key]; ok && r.OffsetMs-at <= copyWindowMs {
			s.RepeatedCopies++
			lastCopy[key] = r.OffsetMs
			continue
		}
		lastCopy[key] = r.OffsetMs
		read(&s, r.Frame, claimed, seenClaim)
	}
	for _, n := range perSecond {
		s.MaxPerSec = max(s.MaxPerSec, n)
	}
	for _, macs := range claimed {
		if len(macs) > 1 {
			s.ConflictingAddresses++
		}
	}
	slices.SortFunc(s.Claims, func(a, b models.CaptureClaim) int {
		if c := compareStrings(a.IP, b.IP); c != 0 {
			return c
		}
		return compareStrings(a.MAC, b.MAC)
	})
	return s, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// read classifies one frame by its first bytes and adds it to the counts.
func read(s *models.CaptureSummary, f []byte, claimed map[string]map[string]bool, seen map[[2]string]bool) {
	if len(f) < ethHeader {
		s.Counts.Unreadable++
		return
	}
	switch binary.BigEndian.Uint16(f[12:14]) {
	case etherTypeARP:
		readARP(s, f, claimed, seen)
	case etherTypeIPv4:
		readIPv4(s, f)
	case etherTypeIPv6:
		readIPv6(s, f)
	default:
		s.Counts.Unreadable++
	}
}

func readARP(s *models.CaptureSummary, f []byte, claimed map[string]map[string]bool, seen map[[2]string]bool) {
	if len(f) < arpLen {
		s.Counts.Unreadable++
		return
	}
	op := binary.BigEndian.Uint16(f[20:22])
	switch op {
	case 1:
		s.Counts.ARPRequests++
	case 2:
		s.Counts.ARPReplies++
	default:
		s.Counts.Unreadable++
		return
	}
	mac := formatMAC(f[22:28])
	spa, tpa := netip.AddrFrom4([4]byte(f[28:32])), netip.AddrFrom4([4]byte(f[38:42]))
	gratuitous := spa == tpa
	if gratuitous {
		s.Counts.ARPGratuitous++
	}
	if spa.IsUnspecified() { // a probe for an address (DHCP duplicate check) announces nothing
		return
	}
	ip := spa.String()
	if claimed[ip] == nil {
		claimed[ip] = map[string]bool{}
	}
	claimed[ip][mac] = true
	key := [2]string{mac, ip}
	if !seen[key] && len(s.Claims) < maxClaims {
		seen[key] = true
		s.Claims = append(s.Claims, models.CaptureClaim{MAC: mac, IP: ip, Gratuitous: gratuitous})
	}
}

// udpPorts returns the UDP source and destination ports of an Ethernet and IPv4 frame, using the header length the frame states. It
// refuses a frame that is too short, not UDP, or a later fragment (which has no UDP header).
func udpPorts(f []byte) (sport, dport uint16, ok bool) {
	if len(f) < ethHeader+ipv4Min {
		return 0, 0, false
	}
	ihl := int(f[ethHeader]&0x0f) * 4
	if ihl < ipv4Min || len(f) < ethHeader+ihl+udpHeader || f[ethHeader+9] != udpProto || binary.BigEndian.Uint16(f[ethHeader+6:ethHeader+8])&fragmentMask != 0 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(f[ethHeader+ihl:]), binary.BigEndian.Uint16(f[ethHeader+ihl+2:]), true
}

func readIPv4(s *models.CaptureSummary, f []byte) {
	sport, dport, ok := udpPorts(f)
	switch {
	case !ok:
		s.Counts.Unreadable++
	case sport == portDHCPSrv && dport == portDHCPCli:
		s.Counts.DHCPFromServer++
		if mac := formatMAC(f[6:12]); !slices.Contains(s.DHCPServerMACs, mac) && len(s.DHCPServerMACs) < maxDHCPServers {
			s.DHCPServerMACs = append(s.DHCPServerMACs, mac)
		}
	case sport == portDHCPCli && dport == portDHCPSrv:
		s.Counts.DHCPFromClient++
	case sport == portMDNS || dport == portMDNS:
		s.Counts.MDNS++
	default:
		s.Counts.Unreadable++
	}
}

func readIPv6(s *models.CaptureSummary, f []byte) {
	if len(f) < ipv6HeaderEnd+1 || f[ethHeader+6] != icmpv6Proto {
		s.Counts.Unreadable++
		return
	}
	switch f[ipv6HeaderEnd] {
	case 133:
		s.Counts.NDRouterSolicit++
	case 134:
		s.Counts.NDRouterAdvert++
	case 135:
		s.Counts.NDNeighborSolicit++
	case 136:
		s.Counts.NDNeighborAdvert++
	case 137:
		s.Counts.NDRedirect++
	default:
		s.Counts.Unreadable++
	}
}

func formatMAC(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 17)
	for i, v := range b[:6] {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[v>>4], hex[v&0xf])
	}
	return string(out)
}

// MarkOffNetwork flags each announced address that is outside every range the link has itself (prefixes). With no known range nothing
// is marked: an unknown link is not evidence of anything. Only IPv4 ranges count, as an ARP announcement is IPv4.
func MarkOffNetwork(s models.CaptureSummary, prefixes []netip.Prefix) models.CaptureSummary {
	prefixes = slices.DeleteFunc(slices.Clone(prefixes), func(p netip.Prefix) bool { return !p.Addr().Is4() }) // ARP announces IPv4 only
	if len(prefixes) == 0 {
		return s
	}
	off := map[string]bool{}
	for i, c := range s.Claims {
		ip, err := netip.ParseAddr(c.IP)
		if err != nil {
			continue
		}
		if !slices.ContainsFunc(prefixes, func(p netip.Prefix) bool { return p.Contains(ip) }) {
			s.Claims[i].OffNetwork = true
			off[c.IP] = true
		}
	}
	s.OffNetworkAddresses = len(off)
	return s
}

// Stamp fills in what the frames cannot say: when the window ran, on which link, and whether it is Wi-Fi.
func Stamp(s models.CaptureSummary, at time.Time, iface string, wireless bool) models.CaptureSummary {
	s.At, s.Interface, s.Wireless = at.UTC(), iface, wireless
	return s
}
