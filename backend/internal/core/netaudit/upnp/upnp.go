// Package upnp asks the home router, and only the router, whether it offers UPnP: the protocol that lets any device on the
// network open a port to the internet by itself. It sends one unicast discovery datagram to the router's address and
// reads its description page. Everything is read-only, goes through the guarded network client, and nothing the router
// writes is kept except a few fixed facts.
package upnp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Service names the router's internet connection service, which holds the port mappings.
type Service string

const (
	ServiceWANIP  Service = "WANIPConnection"
	ServiceWANPPP Service = "WANPPPConnection"
)

const (
	defaultSSDPPort     = 1900
	maxAnswerBytes      = 2048
	maxLocationLen      = 512
	maxDescriptionBytes = 256 << 10
	defaultWait         = 3 * time.Second
	gatewayDeviceType   = "urn:schemas-upnp-org:device:InternetGatewayDevice:"
)

// ErrNotTheRouter means an answer pointed somewhere other than the router, or was not an answer at all.
var ErrNotTheRouter = errors.New("the UPnP answer did not come from the router's own address")

// Dial is the guarded dial function (NetworkTools.DialContext): it refuses addresses the network policy forbids.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Doer fetches the description page: the guarded HTTP client.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client probes one router.
type Client struct {
	Dial Dial
	HTTP Doer
	// SSDPPort is the router's discovery port; zero means 1900. Wait is how long to wait for an answer; zero means three seconds.
	SSDPPort int
	Wait     time.Duration
}

// Report is what the router said, as fixed facts.
type Report struct {
	// Answered is true when the router replied to the discovery request. A router with UPnP off says nothing, so false does
	// not prove UPnP is off.
	Answered bool
	// Gateway is true when the description page names an Internet Gateway Device.
	Gateway bool
	// Service is the router's internet connection service, or empty.
	Service Service
	// ControlPath is where that service takes commands: a path on the router.
	ControlPath string
	// Location is the description page, which is on the router.
	Location string
}

// Probe asks the router at gateway whether it answers UPnP. No answer is not an error. A refused dial, an answer from
// elsewhere or an unreadable page is.
func (c Client) Probe(ctx context.Context, gateway netip.Addr) (Report, error) {
	raw, err := c.discover(ctx, gateway)
	if err != nil || raw == nil {
		return Report{}, err
	}
	location, err := ParseSSDP(raw, gateway)
	if err != nil {
		return Report{}, err
	}
	desc, err := c.describe(ctx, location)
	if err != nil {
		return Report{Answered: true, Location: location}, err
	}
	return Report{Answered: true, Gateway: desc.Gateway, Service: desc.Service, ControlPath: desc.ControlPath, Location: location}, nil
}

// discover sends one M-SEARCH to the router and returns its answer, or nil when none came in time.
func (c Client) discover(ctx context.Context, gateway netip.Addr) ([]byte, error) {
	port := c.SSDPPort
	if port == 0 {
		port = defaultSSDPPort
	}
	target := net.JoinHostPort(gateway.String(), strconv.Itoa(port))
	conn, err := c.Dial(ctx, "udp", target)
	if err != nil {
		return nil, fmt.Errorf("could not reach the router for UPnP: %w", err)
	}
	defer conn.Close()
	wait := c.Wait
	if wait == 0 {
		wait = defaultWait
	}
	_ = conn.SetDeadline(time.Now().Add(wait))
	search := "M-SEARCH * HTTP/1.1\r\nHOST: " + target + "\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: upnp:rootdevice\r\n\r\n"
	if _, err := conn.Write([]byte(search)); err != nil {
		return nil, fmt.Errorf("could not send the UPnP request: %w", err)
	}
	buf := make([]byte, maxAnswerBytes)
	n, err := conn.Read(buf)
	if err != nil {
		var ne net.Error
		if (errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, nil // nothing answered, or nothing is listening (ICMP port unreachable): the same as no answer
		}
		return nil, fmt.Errorf("could not read the router's UPnP answer: %w", err)
	}
	return buf[:n], nil
}

// ParseSSDP reads a discovery answer and returns the description page's address. It must be a plain 200 answer whose
// LOCATION is an http address on the router itself, as an IP literal with a path: an answer that sends the reader to
// another host is how a hostile device would turn this probe into a request somewhere else.
func ParseSSDP(raw []byte, gateway netip.Addr) (string, error) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "HTTP/1.1 200") {
		return "", fmt.Errorf("%w: not a 200 answer", ErrNotTheRouter)
	}
	location := ""
	for _, l := range lines[1:] {
		if name, value, ok := strings.Cut(l, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "LOCATION") {
			location = strings.TrimSpace(value)
			break
		}
	}
	return checkLocation(location, gateway)
}

func checkLocation(location string, gateway netip.Addr) (string, error) {
	if location == "" || len(location) > maxLocationLen {
		return "", fmt.Errorf("%w: no usable LOCATION", ErrNotTheRouter)
	}
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/") {
		return "", fmt.Errorf("%w: unexpected LOCATION", ErrNotTheRouter)
	}
	host, err := netip.ParseAddr(u.Hostname())
	if err != nil || host.Unmap() != gateway.Unmap() {
		return "", fmt.Errorf("%w: LOCATION is another host", ErrNotTheRouter)
	}
	return location, nil
}

// describe fetches the description page, which is on the router, without following a redirect.
func (c Client) describe(ctx context.Context, location string) (Description, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*defaultWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return Description{}, err
	}
	resp, err := noRedirects(c.HTTP).Do(req)
	if err != nil {
		return Description{}, fmt.Errorf("could not read the router's UPnP page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Description{}, fmt.Errorf("the router's UPnP page answered %d", resp.StatusCode)
	}
	return ParseDescription(resp.Body)
}

// noRedirects wraps a client so a redirect is an answer, not a journey: the page must be on the router.
func noRedirects(d Doer) Doer {
	if hc, ok := d.(*http.Client); ok {
		copied := *hc
		copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return &copied
	}
	return d
}

// Description is what the router's page says, reduced to fixed facts.
type Description struct {
	Gateway     bool
	Service     Service
	ControlPath string
}

type xmlDevice struct {
	DeviceType string       `xml:"deviceType"`
	Services   []xmlService `xml:"serviceList>service"`
	Devices    []xmlDevice  `xml:"deviceList>device"`
}

type xmlService struct {
	Type    string `xml:"serviceType"`
	Control string `xml:"controlURL"`
}

// ParseDescription reads a router's description page: whether it is an Internet Gateway Device and where its internet
// connection service takes commands. Nothing else is kept. The page is read through a size limit; a control URL that is
// not a plain path on the router is ignored.
func ParseDescription(r io.Reader) (Description, error) {
	lr := &io.LimitedReader{R: r, N: maxDescriptionBytes + 1}
	var root struct {
		Device xmlDevice `xml:"device"`
	}
	if err := xml.NewDecoder(lr).Decode(&root); err != nil {
		return Description{}, fmt.Errorf("the router's UPnP page could not be read: %w", err)
	}
	if lr.N <= 0 {
		return Description{}, errors.New("the router's UPnP page is larger than the limit")
	}
	d := Description{Gateway: strings.HasPrefix(root.Device.DeviceType, gatewayDeviceType)}
	if d.Gateway {
		d.Service, d.ControlPath = findConnection(root.Device)
	}
	return d, nil
}

// findConnection searches the device tree for the internet connection service.
func findConnection(dev xmlDevice) (Service, string) {
	for _, s := range dev.Services {
		for _, want := range []Service{ServiceWANIP, ServiceWANPPP} {
			if strings.HasPrefix(s.Type, "urn:schemas-upnp-org:service:"+string(want)+":") && plainPath(s.Control) {
				return want, s.Control
			}
		}
	}
	for _, child := range dev.Devices {
		if svc, path := findConnection(child); svc != "" {
			return svc, path
		}
	}
	return "", ""
}

func plainPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.ContainsAny(p, "?#\\ \r\n") && len(p) <= maxLocationLen
}
