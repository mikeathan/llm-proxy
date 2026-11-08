package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"llm-proxy-stacks/sdk"
)

// ErrInternetOff means the network settings do not allow the request, so the outside resolver was not asked.
var ErrInternetOff = errors.New("the network settings do not allow internet access")

// maxBody bounds how much of an answer is read.
const maxBody = 16 << 10

// dohAccept names the JSON form of a DNS-over-HTTPS answer.
const dohAccept = "application/dns-json"

// Outside asks an outside DNS-over-HTTPS resolver, over HTTPS through the core's guarded fetch. URL is read on every
// question, so a change of setting takes effect at once; an empty value means DefaultResolverURL.
type Outside struct {
	Fetch stacksdk.Fetch
	URL   func() string
}

// ValidateResolverURL checks a resolver address typed by the operator: https only, a host, no user name, query or fragment.
func ValidateResolverURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", errors.New("the resolver address must start with https:// and name a host, with no user name, query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// LookupA returns the IPv4 addresses the resolver gives for name. Every failure is an error, never an empty answer.
func (o Outside) LookupA(ctx context.Context, name string) ([]netip.Addr, error) {
	names, err := ValidateNames([]string{name})
	if err != nil {
		return nil, err
	}
	base := DefaultResolverURL
	if o.URL != nil {
		if set := strings.TrimSpace(o.URL()); set != "" {
			base = set
		}
	}
	if base, err = ValidateResolverURL(base); err != nil {
		return nil, err
	}
	resp, err := o.Fetch.Get(ctx, stacksdk.FetchRequest{URL: base + "?name=" + url.QueryEscape(names[0]) + "&type=A", Accept: dohAccept, MaxBytes: maxBody})
	if stacksdk.CodeOf(err) == stacksdk.CodeUnavailable {
		return nil, ErrInternetOff
	}
	if err != nil {
		return nil, fmt.Errorf("the outside resolver could not be asked: %w", err)
	}
	return ParseDoH(resp.Body)
}
