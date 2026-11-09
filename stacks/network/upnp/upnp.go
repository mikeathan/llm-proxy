// Package upnp asks the home router, and only the router, whether it offers UPnP: the protocol that lets any device on the
// network open a port to the internet by itself. It sends one unicast discovery datagram to the router's address and
// reads its description page, its list of forwarded ports and, when asked, the address of its internet side. Everything is
// read-only and every connection is made by the dial it is given (in the Network stack, the host's scoped dial), and nothing
// the router writes is kept except a few fixed facts, as numbers and ids (ADR-0038 rule 3).
package upnp

import (
	"bufio"
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

const (
	defaultSSDPPort     = 1900
	maxAnswerBytes      = 2048
	maxLocationLen      = 512
	maxDescriptionBytes = 256 << 10
	maxHeaderBytes      = 64 << 10
	defaultWait         = 3 * time.Second
	dialTimeout         = 5 * time.Second
	gatewayDeviceType   = "urn:schemas-upnp-org:device:InternetGatewayDevice:"
	userAgent           = "llm-proxy-network-audit"
)

// The reasons a router could not be read, as ids. The core owns the words for each (ADR-0038 rule 3); nothing the router said is in them.
const (
	ProblemUnreachable = "unreachable" // the datagram could not be sent or its answer could not be read
	ProblemNotRouter   = "not_router"  // the answer was not a plain answer naming a page on the router itself
	ProblemPage        = "page_failed" // the description page could not be fetched or read
	// ProblemList and the three after it are why the list of forwarded ports could not be read.
	ProblemList       = "list_failed"
	ProblemListRefuse = "list_refused"
	ProblemListRead   = "list_unreadable"
	ProblemListUnused = "list_unusable"
)

// Problems lists every id above, so a test can hold the core's table to it.
func Problems() []string {
	return []string{ProblemUnreachable, ProblemNotRouter, ProblemPage, ProblemList, ProblemListRefuse, ProblemListRead, ProblemListUnused}
}

// problem is an error that carries one of the ids above.
type problem struct {
	id  string
	err error
}

func (p problem) Error() string { return p.id + ": " + p.err.Error() }
func (p problem) Unwrap() error { return p.err }

func fail(id string, err error) error { return problem{id: id, err: err} }

// ProblemOf returns the id an error carries, or ProblemUnreachable for one that carries none.
func ProblemOf(err error) string {
	var p problem
	if errors.As(err, &p) {
		return p.id
	}
	return ProblemUnreachable
}

// ErrNotTheRouter means an answer pointed somewhere other than the router, or was not an answer at all.
var ErrNotTheRouter = errors.New("the UPnP answer did not come from the router's own address")

// Dial opens one connection: in the Network stack, the host's scoped dial.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Client probes one router.
type Client struct {
	Dial Dial
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
	// HasService is true when the page names an internet connection service, which holds the port mappings.
	HasService bool
	// ControlPath is where that service takes commands: a path on the router. ServiceType is the service's full name, checked
	// against a fixed pattern before it is ever sent back.
	ControlPath string
	ServiceType string
	// Mappings are the ports the router forwards to devices on the network, enabled or not. MappingsProblem is the id of why they could
	// not be listed; the rest of the report still stands.
	Mappings        []Mapping
	MappingsProblem string
	// Location is the description page, which is on the router.
	Location string
}

// Look asks the router at gateway whether it answers UPnP, reads its forwarded ports and, when wantAddress is set and the router has an internet
// connection service, the address of its internet side (the zero Addr when it could not be read). No answer is not an error. A refused dial, an
// answer from elsewhere or an unreadable page is, and carries a problem id.
func (c Client) Look(ctx context.Context, gateway netip.Addr, wantAddress bool) (Report, netip.Addr, error) {
	report, err := c.Probe(ctx, gateway)
	if err != nil || !report.Answered || !report.Gateway || !report.HasService || !wantAddress {
		return report, netip.Addr{}, err
	}
	addr, aerr := c.ExternalIP(ctx, report.Location, report.ControlPath, report.ServiceType)
	if aerr != nil {
		return report, netip.Addr{}, nil
	}
	return report, addr, nil
}

// Probe asks the router at gateway whether it answers UPnP. No answer is not an error.
func (c Client) Probe(ctx context.Context, gateway netip.Addr) (Report, error) {
	raw, err := c.discover(ctx, gateway)
	if err != nil || raw == nil {
		return Report{}, err
	}
	location, err := ParseSSDP(raw, gateway)
	if err != nil {
		return Report{}, fail(ProblemNotRouter, err)
	}
	desc, err := c.describe(ctx, location)
	if err != nil {
		return Report{Answered: true, Location: location}, fail(ProblemPage, err)
	}
	report := Report{Answered: true, Gateway: desc.Gateway, HasService: desc.HasService, ControlPath: desc.ControlPath, ServiceType: desc.ServiceType, Location: location}
	if desc.HasService {
		maps, err := c.Mappings(ctx, location, desc.ControlPath, desc.ServiceType)
		report.Mappings = maps
		if err != nil {
			report.MappingsProblem = ProblemOf(err)
		}
	}
	return report, nil
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
		return nil, fail(ProblemUnreachable, fmt.Errorf("could not reach the router for UPnP: %w", err))
	}
	defer conn.Close()
	wait := c.Wait
	if wait == 0 {
		wait = defaultWait
	}
	_ = conn.SetDeadline(time.Now().Add(wait))
	search := "M-SEARCH * HTTP/1.1\r\nHOST: " + target + "\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: upnp:rootdevice\r\n\r\n"
	if _, err := conn.Write([]byte(search)); err != nil {
		return nil, fail(ProblemUnreachable, fmt.Errorf("could not send the UPnP request: %w", err))
	}
	buf := make([]byte, maxAnswerBytes)
	n, err := conn.Read(buf)
	if err != nil {
		var ne net.Error
		if (errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, nil // nothing answered, or nothing is listening (ICMP port unreachable): the same as no answer
		}
		return nil, fail(ProblemUnreachable, fmt.Errorf("could not read the router's UPnP answer: %w", err))
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

// describe fetches the description page, which is on the router. A redirect is an answer, never followed.
func (c Client) describe(ctx context.Context, location string) (Description, error) {
	u, err := url.Parse(location)
	if err != nil {
		return Description{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*defaultWait)
	defer cancel()
	status, body, err := c.exchange(ctx, http.MethodGet, u.Host, u.Path, nil, "", maxDescriptionBytes+1)
	if err != nil {
		return Description{}, fmt.Errorf("could not read the UPnP page: %w", err)
	}
	if status != http.StatusOK {
		return Description{}, fmt.Errorf("the UPnP page answered %d", status)
	}
	return ParseDescription(strings.NewReader(string(body)))
}

// exchange writes one plain request on a connection made by the dial and reads one response from it, and so has no way to follow a redirect, use
// a proxy, keep a connection or connect anywhere else: the request names the router's address and the connection is closed after the answer. The
// answer is returned whatever its status, because a UPnP fault arrives as a 500 with a body the caller must read. Everything read is bounded: the
// headers by maxHeaderBytes, the body by limit, and the whole exchange ends when ctx does.
func (c Client) exchange(ctx context.Context, method, hostport, path string, header http.Header, body string, limit int64) (int, []byte, error) {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, err := c.Dial(dctx, "tcp", hostport)
	if err != nil {
		return 0, nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+hostport+path, strings.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	req.Close = true // sends "Connection: close"
	if err := req.Write(conn); err != nil {
		return 0, nil, err
	}
	resp, err := http.ReadResponse(bufio.NewReaderSize(io.LimitReader(conn, maxHeaderBytes+limit), maxHeaderBytes), req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil && len(data) == 0 {
		return 0, nil, err
	}
	return resp.StatusCode, data, nil
}

// Description is what the router's page says, reduced to fixed facts.
type Description struct {
	Gateway     bool
	HasService  bool
	ControlPath string
	ServiceType string
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
		d.HasService, d.ControlPath, d.ServiceType = findConnection(root.Device)
	}
	return d, nil
}

// findConnection searches the device tree for the internet connection service and returns its control path and its full service type.
func findConnection(dev xmlDevice) (bool, string, string) {
	for _, s := range dev.Services {
		for _, want := range []string{"WANIPConnection", "WANPPPConnection"} {
			if strings.HasPrefix(s.Type, "urn:schemas-upnp-org:service:"+want+":") && plainPath(s.Control) {
				return true, s.Control, s.Type
			}
		}
	}
	for _, child := range dev.Devices {
		if ok, path, urn := findConnection(child); ok {
			return true, path, urn
		}
	}
	return false, "", ""
}

func plainPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.ContainsAny(p, "?#\\ \r\n") && len(p) <= maxLocationLen
}
