package upnp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxMappings bounds the list read from the router: one request per entry.
	MaxMappings    = 64
	maxSOAPBytes   = 64 << 10
	mappingWait    = 3 * time.Second
	errEndOfList   = 713 // SpecifiedArrayIndexInvalid: the UPnP answer for "there is no entry with that index"
	mappingAction  = "GetGenericPortMappingEntry"
	maxPort        = 65535
	protocolTCP    = "tcp"
	protocolUDP    = "udp"
	enabledMapping = "1"
)

// serviceURN is the only shape of service type that is put into a request header: the router's own text must not be able to
// add a header line.
var serviceURN = regexp.MustCompile(`^urn:schemas-upnp-org:service:WAN(IP|PPP)Connection:[12]$`)

// Mapping is one port the router forwards from the internet side to a device on the network.
type Mapping struct {
	ExternalPort int
	InternalPort int
	// Protocol is "tcp" or "udp".
	Protocol string
	// Client is the IPv4 address of the device the traffic is forwarded to.
	Client  netip.Addr
	Enabled bool
}

// Mappings asks the router for its list of forwarded ports, one entry at a time, until it says there are no more. The
// request is a read; the router's own description text for an entry is never read at all. location is the description page
// (it names the router's address and port) and controlPath where the connection service takes commands. A failure carries a problem id.
func (c Client) Mappings(ctx context.Context, location, controlPath, serviceType string) ([]Mapping, error) {
	hostport, path, err := controlEndpoint(location, controlPath, serviceType)
	if err != nil {
		return nil, fail(ProblemListUnused, err)
	}
	var out []Mapping
	for index := 0; index < MaxMappings; index++ {
		m, end, err := c.entry(ctx, hostport, path, serviceType, index)
		if err != nil {
			return out, err
		}
		if end {
			return out, nil
		}
		if m != nil {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (c Client) entry(ctx context.Context, hostport, path, serviceType string, index int) (*Mapping, bool, error) {
	raw, err := c.soapCall(ctx, hostport, path, serviceType, mappingAction, "<NewPortMappingIndex>"+strconv.Itoa(index)+"</NewPortMappingIndex>", mappingWait)
	if err != nil {
		return nil, false, fail(ProblemList, fmt.Errorf("could not list the router's forwarded ports: %w", err))
	}
	return parseEntry(raw)
}

type soapEnvelope struct {
	Body struct {
		Entry struct {
			ExternalPort string `xml:"NewExternalPort"`
			Protocol     string `xml:"NewProtocol"`
			InternalPort string `xml:"NewInternalPort"`
			Client       string `xml:"NewInternalClient"`
			Enabled      string `xml:"NewEnabled"`
		} `xml:"GetGenericPortMappingEntryResponse"`
		Fault struct {
			Code string `xml:"detail>UPnPError>errorCode"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// parseEntry reads the router's answer to one GetGenericPortMappingEntry. It returns the mapping, or end=true when the
// router says there is no entry at that index (UPnP error 713). Any other answer that is not a well-formed mapping is an
// error: a list that could not be read is never reported as empty. An entry with a port out of range, an unknown protocol
// or a client that is not an IPv4 address is skipped, not trusted.
func parseEntry(raw []byte) (m *Mapping, end bool, err error) {
	var env soapEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, false, fail(ProblemListRead, fmt.Errorf("the router's list of forwarded ports could not be read: %w", err))
	}
	if code := strings.TrimSpace(env.Body.Fault.Code); code != "" {
		if n, _ := strconv.Atoi(code); n == errEndOfList {
			return nil, true, nil
		}
		return nil, false, fail(ProblemListRefuse, errors.New("the router refused to list its forwarded ports"))
	}
	e := env.Body.Entry
	if e.ExternalPort == "" && e.InternalPort == "" && e.Client == "" {
		return nil, false, fail(ProblemListRead, errors.New("the router's answer held no forwarded port and no end of list"))
	}
	return validMapping(e.ExternalPort, e.Protocol, e.InternalPort, e.Client, e.Enabled), false, nil
}

// validMapping checks the fields of one entry and returns the mapping, or nil for an entry that cannot be trusted: a port out
// of range, a protocol other than tcp or udp, or a client that is not an IPv4 address.
func validMapping(external, protocol, internal, client, enabled string) *Mapping {
	ext, in := atoiPort(external), atoiPort(internal)
	proto := strings.ToLower(strings.TrimSpace(protocol))
	addr, err := netip.ParseAddr(strings.TrimSpace(client))
	if ext == 0 || in == 0 || (proto != protocolTCP && proto != protocolUDP) || err != nil || !addr.Is4() {
		return nil
	}
	return &Mapping{ExternalPort: ext, InternalPort: in, Protocol: proto, Client: addr, Enabled: strings.TrimSpace(enabled) == enabledMapping}
}

func atoiPort(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > maxPort {
		return 0
	}
	return n
}

// controlEndpoint builds the host and path commands are sent to from the description page's host and the control path, after
// checking the service type is one of the fixed shapes (the router's own text must not be able to add a header line).
func controlEndpoint(location, controlPath, serviceType string) (hostport, path string, err error) {
	if !serviceURN.MatchString(serviceType) {
		return "", "", errors.New("the router named a connection service this program does not recognise")
	}
	base, err := url.Parse(location)
	if err != nil || base.Host == "" || !plainPath(controlPath) {
		return "", "", errors.New("the router's UPnP control address is not usable")
	}
	return base.Host, controlPath, nil
}

// soapCall sends one action to the router's control address and returns the reply body, read up to a fixed size. The reply is
// returned whatever the HTTP status is, because a UPnP fault (such as "no entry at that index") arrives as a 500 with a body
// the caller must read.
func (c Client) soapCall(ctx context.Context, hostport, path, serviceType, action, args string, wait time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` +
		action + ` xmlns:u="` + serviceType + `">` + args + `</u:` + action + `></s:Body></s:Envelope>`
	header := http.Header{"Content-Type": {`text/xml; charset="utf-8"`}, "Soapaction": {`"` + serviceType + `#` + action + `"`}}
	_, data, err := c.exchange(ctx, http.MethodPost, hostport, path, header, body, maxSOAPBytes)
	return data, err
}
