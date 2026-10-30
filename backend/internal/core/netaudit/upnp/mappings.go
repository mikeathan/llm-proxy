package upnp

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// maxMappings bounds the list read from the router: one request per entry.
	maxMappings    = 64
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
// (it names the router's address and port) and controlPath where the connection service takes commands.
func (c Client) Mappings(ctx context.Context, location, controlPath, serviceType string) ([]Mapping, error) {
	if !serviceURN.MatchString(serviceType) {
		return nil, errors.New("the router named a connection service this program does not recognise")
	}
	base, err := url.Parse(location)
	if err != nil || base.Host == "" || !plainPath(controlPath) {
		return nil, errors.New("the router's UPnP control address is not usable")
	}
	endpoint := (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: controlPath}).String()
	var out []Mapping
	for index := 0; index < maxMappings; index++ {
		m, end, err := c.entry(ctx, endpoint, serviceType, index)
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

func (c Client) entry(ctx context.Context, endpoint, serviceType string, index int) (*Mapping, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, mappingWait)
	defer cancel()
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` +
		mappingAction + ` xmlns:u="` + serviceType + `"><NewPortMappingIndex>` + strconv.Itoa(index) + `</NewPortMappingIndex></u:` + mappingAction + `></s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+serviceType+`#`+mappingAction+`"`)
	resp, err := noRedirects(c.HTTP).Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("could not list the router's forwarded ports: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSOAPBytes))
	if err != nil {
		return nil, false, fmt.Errorf("could not list the router's forwarded ports: %w", err)
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
		return nil, false, fmt.Errorf("the router's list of forwarded ports could not be read: %w", err)
	}
	if code := strings.TrimSpace(env.Body.Fault.Code); code != "" {
		if n, _ := strconv.Atoi(code); n == errEndOfList {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("the router refused to list its forwarded ports (UPnP error %.4s)", code)
	}
	e := env.Body.Entry
	if e.ExternalPort == "" && e.InternalPort == "" && e.Client == "" {
		return nil, false, errors.New("the router's answer held no forwarded port and no end of list")
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
