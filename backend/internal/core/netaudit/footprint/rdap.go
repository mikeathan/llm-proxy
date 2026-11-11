package footprint

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"
)

// BootstrapURL is IANA's list of which registry serves which domain ending (RFC 9224).
const BootstrapURL = "https://data.iana.org/rdap/dns.json"

const (
	maxBootstrap  = 1 << 20
	maxRDAP       = 256 << 10
	bootstrapKeep = 24 * time.Hour
)

// Registry finds a domain's registry through IANA's bootstrap list and reads its expiry date. The list is kept for a day, so one
// look at a domain is one request to the registry and, at most once a day, one to IANA.
type Registry struct {
	Opener Opener
	Now    func() time.Time

	mu      sync.Mutex
	loaded  time.Time
	servers map[string]string // domain ending -> registry base URL (https only)
}

// Expiry returns when the domain's registration ends, as its registry states.
func (r *Registry) Expiry(ctx context.Context, domain string) (time.Time, error) {
	base, err := r.serverFor(ctx, domain)
	if err != nil {
		return time.Time{}, err
	}
	body, err := get(ctx, r.Opener, base+"domain/"+url.PathEscape(domain), "application/rdap+json", nil, maxRDAP)
	if err != nil {
		return time.Time{}, err
	}
	var answer struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return time.Time{}, &LookError{Reason: ReasonUnreadable}
	}
	for _, e := range answer.Events {
		if e.Action != "expiration" {
			continue
		}
		at, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			return time.Time{}, &LookError{Reason: ReasonUnreadable}
		}
		return at.UTC(), nil
	}
	return time.Time{}, &LookError{Reason: ReasonNoExpiry}
}

func (r *Registry) serverFor(ctx context.Context, domain string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.servers == nil || now.Sub(r.loaded) > bootstrapKeep {
		servers, err := r.loadBootstrap(ctx)
		if err != nil {
			return "", err
		}
		r.servers, r.loaded = servers, now
	}
	ending := domain[strings.LastIndex(domain, ".")+1:]
	base, ok := r.servers[ending]
	if !ok {
		return "", &LookError{Reason: ReasonNoRegistry}
	}
	return base, nil
}

func (r *Registry) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// loadBootstrap reads the list and keeps, for each ending, the first registry address that is https. A registry listed only with
// plain http is left out: the answer would travel unprotected, so the look reads "no registry" rather than downgrade.
func (r *Registry) loadBootstrap(ctx context.Context) (map[string]string, error) {
	body, err := get(ctx, r.Opener, BootstrapURL, "application/json", nil, maxBootstrap)
	if err != nil {
		return nil, err
	}
	var list struct {
		Services [][][]string `json:"services"`
	}
	if json.Unmarshal(body, &list) != nil {
		return nil, &LookError{Reason: ReasonUnreadable}
	}
	out := map[string]string{}
	for _, svc := range list.Services {
		if len(svc) != 2 {
			continue
		}
		base := firstHTTPS(svc[1])
		if base == "" {
			continue
		}
		for _, ending := range svc[0] {
			out[strings.ToLower(ending)] = base
		}
	}
	return out, nil
}

func firstHTTPS(urls []string) string {
	for _, u := range urls {
		if !strings.HasPrefix(u, "https://") {
			continue
		}
		if !strings.HasSuffix(u, "/") {
			u += "/"
		}
		if parsed, err := url.Parse(u); err == nil && parsed.Hostname() != "" && parsed.User == nil {
			return u
		}
	}
	return ""
}
