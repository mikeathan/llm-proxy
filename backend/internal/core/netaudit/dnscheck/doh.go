package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	outside "llm-proxy/internal/core/netaudit/outsidelookup"
	"llm-proxy/internal/core/tools"
)

// ErrInternetOff means the network settings do not allow the request, so the outside resolver was not asked.
var ErrInternetOff = outside.ErrInternetOff

// maxBody bounds how much of an answer is read.
const maxBody = 16 << 10

// dohAccept names the JSON form of a DNS-over-HTTPS answer.
const dohAccept = "application/dns-json"

// Opener makes a guarded GET that asks for one format: *tools.NetworkTools.
type Opener interface {
	OpenAccepting(ctx context.Context, target, accept string) (*http.Response, error)
}

// Outside asks an outside DNS-over-HTTPS resolver, over HTTPS through the guarded network client. URL is read on every
// question, so a change of setting takes effect at once; an empty value means DefaultResolverURL.
type Outside struct {
	Opener Opener
	URL    func() string
}

// ValidateResolverURL checks a resolver address typed by the operator: https only, a host, no user name, query or fragment.
func ValidateResolverURL(raw string) (string, error) {
	kept, err := outside.ValidateBaseURL(raw)
	if err != nil {
		return "", errors.New("the resolver address must start with https:// and name a host, with no user name, query or fragment")
	}
	return kept, nil
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
	resp, err := o.Opener.OpenAccepting(ctx, base+"?name="+url.QueryEscape(names[0])+"&type=A", dohAccept)
	if errors.Is(err, tools.ErrNetworkDisabled) || errors.Is(err, tools.ErrInternetBlocked) {
		return nil, ErrInternetOff
	}
	if err != nil {
		return nil, fmt.Errorf("the outside resolver could not be asked: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("the outside resolver's answer could not be read: %w", err)
	}
	return ParseDoH(raw)
}
