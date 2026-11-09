// Package upnp reads what one UPnP device says about itself, for the Nearby collector: the address of a device's description page from its
// discovery answer, and the page's fixed facts. The router check (the probe of the router, its forwarded ports and its internet address) is
// the Network stack's sensor (stacks/network/upnp, ADR-0038); this package keeps only what Nearby still calls, and goes when Nearby moves.
package upnp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	maxLocationLen      = 512
	maxDescriptionBytes = 256 << 10
	fetchWait           = 6 * time.Second
	gatewayDeviceType   = "urn:schemas-upnp-org:device:InternetGatewayDevice:"
)

// ErrNotTheRouter means an answer pointed somewhere other than the device that sent it, or was not an answer at all.
var ErrNotTheRouter = errors.New("the UPnP answer did not come from the router's own address")

// Doer fetches the description page: the guarded HTTP client.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client reads one device's description page.
type Client struct {
	HTTP Doer
}

// ParseSSDP reads a discovery answer and returns the description page's address. It must be a plain 200 answer whose
// LOCATION is an http address on the replier itself, as an IP literal with a path: an answer that sends the reader to
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

// fetchPage reads one description page through the guarded client without following a redirect and parses it.
func fetchPage[T any](ctx context.Context, doer Doer, location string, parse func(io.Reader) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, fetchWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return zero, err
	}
	resp, err := noRedirects(doer).Do(req)
	if err != nil {
		return zero, fmt.Errorf("could not read the UPnP page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("the UPnP page answered %d", resp.StatusCode)
	}
	return parse(resp.Body)
}

// noRedirects wraps a client so a redirect is an answer, not a journey: the page must be on the device.
func noRedirects(d Doer) Doer {
	if hc, ok := d.(*http.Client); ok {
		copied := *hc
		copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return &copied
	}
	return d
}
