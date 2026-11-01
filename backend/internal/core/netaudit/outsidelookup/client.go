// Package outsidelookup asks an outside scan database which ports it has seen open on this network's public address. It sends one
// HTTPS request per look, through the guarded network client, and reads nothing from the answer except a list of port
// numbers: everything else the service says is its own guesswork or outside text and is never kept.
package outsidelookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"llm-proxy/internal/core/tools"
)

// DefaultURL is the service used when none is set: Shodan's InternetDB, which needs no key and is free for non-commercial use.
const DefaultURL = "https://internetdb.shodan.io"

const (
	// MaxPorts bounds how many ports are read from one answer.
	MaxPorts  = 256
	maxBody   = 64 << 10
	maxPort   = 65535
	noDataGet = http.StatusNotFound
)

// Opener makes a guarded GET: *tools.NetworkTools.
type Opener interface {
	Open(ctx context.Context, target string) (*http.Response, error)
}

// Client looks up one address. BaseURL is read on every look, so a change of setting takes effect at once; an empty value
// means DefaultURL.
type Client struct {
	Opener  Opener
	BaseURL func() string
}

// ErrNotPublic is returned for an address that must never be sent to an outside service.
var ErrNotPublic = errors.New("the address is not a public one, so it is not sent to an outside service")

// ErrInternetOff means the network settings do not allow the request: the network tools are off, or internet access is.
var ErrInternetOff = errors.New("the network settings do not allow internet access")

// ValidateBaseURL checks a service address typed by the operator: https only, a host, no user name, query or fragment. It
// returns the address without a trailing slash.
func ValidateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", errors.New("the service address must start with https:// and name a host, with no user name, query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// OpenPorts returns the ports the service lists as open on addr, sorted, each once. A service with no data on the address (it
// answers 404) gives an empty list: that is its answer, not a failure. Any other trouble is an error, never an empty list.
func (c Client) OpenPorts(ctx context.Context, addr netip.Addr) ([]int, error) {
	if Classify(addr) != Public {
		return nil, ErrNotPublic
	}
	base := DefaultURL
	if c.BaseURL != nil {
		if configured := strings.TrimSpace(c.BaseURL()); configured != "" {
			base = configured
		}
	}
	base, err := ValidateBaseURL(base)
	if err != nil {
		return nil, err
	}
	resp, err := c.Opener.Open(ctx, base+"/"+addr.String())
	var status *tools.StatusError
	if errors.As(err, &status) && status.Code == noDataGet {
		return nil, nil
	}
	if errors.Is(err, tools.ErrNetworkDisabled) || errors.Is(err, tools.ErrInternetBlocked) {
		return nil, ErrInternetOff
	}
	if err != nil {
		return nil, fmt.Errorf("the outside service could not be asked: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("the outside service's answer could not be read: %w", err)
	}
	return parsePorts(raw)
}

// parsePorts reads the "ports" list of an answer. A missing list is an error (the service changed its answer, or this is not
// the service); an entry that is not a port number is skipped.
func parsePorts(raw []byte) ([]int, error) {
	var body struct {
		Ports *[]json.RawMessage `json:"ports"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Ports == nil {
		return nil, errors.New("the outside service's answer did not hold a list of ports")
	}
	var out []int
	for _, item := range *body.Ports {
		var n int
		if json.Unmarshal(item, &n) != nil || n < 1 || n > maxPort {
			continue
		}
		out = append(out, n)
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > MaxPorts {
		out = out[:MaxPorts]
	}
	return out, nil
}
