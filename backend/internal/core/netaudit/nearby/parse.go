// Package nearby learns what the devices on the network say they are, over mDNS and SSDP (Constitution I.9). It sends two fixed
// multicast datagrams, reads a bounded set of unicast replies, parses them into a few typed facts and keeps those. Everything a
// device sends is untrusted: names are cleaned and cut, service types must match a fixed pattern, and nothing else is kept.
package nearby

import (
	"encoding/binary"
	"errors"
	"regexp"
	"slices"
	"strings"

	"llm-proxy/internal/core/netaudit/dnswire"
	"llm-proxy/internal/core/netaudit/names"
)

// Caps on what is kept per device.
const (
	maxServices = 16
	maxNames    = 3
	maxPorts    = 8
)

const serviceEnumeration = "_services._dns-sd._udp.local"

var (
	serviceRe = regexp.MustCompile(`^_[a-z0-9-]{1,15}\._(tcp|udp)$`)
	// inOwner finds the service type at the end of a record's owner name: "<instance>._ipp._tcp.local".
	inOwner = regexp.MustCompile(`(_[a-z0-9-]{1,15}\._(?:tcp|udp))\.local$`)

	errNotAnswer = errors.New("not a usable mDNS answer")
)

// Facts are what one mDNS datagram taught us about its sender.
type Facts struct {
	Services []string
	Names    []string
	Ports    []int
}

// ParseMDNS reads one reply. Only service types, instance names and service ports are kept; TXT records, addresses and
// everything else are dropped while parsing. A message that breaks off keeps what was read before it.
func ParseMDNS(raw []byte) (Facts, error) {
	h, err := dnswire.ParseHeader(raw)
	if err != nil || !h.IsSuccessfulResponse() {
		return Facts{}, errNotAnswer
	}
	recs, _ := dnswire.Records(raw)
	var f Facts
	for _, r := range recs {
		if r.Class != dnswire.ClassIN {
			continue
		}
		switch r.Type {
		case dnswire.TypePTR:
			f.addPTR(raw, r)
		case dnswire.TypeSRV:
			f.addSRV(raw, r)
		}
	}
	if len(f.Services) == 0 && len(f.Names) == 0 && len(f.Ports) == 0 {
		return Facts{}, errNotAnswer
	}
	return f, nil
}

func (f *Facts) addPTR(raw []byte, r dnswire.Record) {
	target, _, err := dnswire.ReadName(raw, r.Data)
	if err != nil {
		return
	}
	owner := strings.ToLower(r.Owner)
	if owner == serviceEnumeration {
		f.addService(strings.TrimSuffix(strings.ToLower(target), ".local"))
		return
	}
	svc := strings.TrimSuffix(owner, ".local")
	if !serviceRe.MatchString(svc) {
		return
	}
	f.addService(svc)
	f.addName(strings.TrimSuffix(target, "."+r.Owner))
}

func (f *Facts) addSRV(raw []byte, r dnswire.Record) {
	if r.Length < 7 {
		return
	}
	m := inOwner.FindStringSubmatch(strings.ToLower(r.Owner))
	if m == nil {
		return
	}
	f.addService(m[1])
	if port := int(binary.BigEndian.Uint16(raw[r.Data+4:])); port > 0 && len(f.Ports) < maxPorts && !slices.Contains(f.Ports, port) {
		f.Ports = append(f.Ports, port)
	}
}

func (f *Facts) addService(s string) {
	if serviceRe.MatchString(s) && len(f.Services) < maxServices && !slices.Contains(f.Services, s) {
		f.Services = append(f.Services, s)
	}
}

func (f *Facts) addName(n string) {
	if n = names.Clean(n); n != "" && len(f.Names) < maxNames && !slices.Contains(f.Names, n) {
		f.Names = append(f.Names, n)
	}
}
