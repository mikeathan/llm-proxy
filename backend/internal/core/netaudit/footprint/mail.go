package footprint

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// DefaultResolverURL is the DNS-over-HTTPS resolver asked for a domain's mail records. It learns the domain.
const DefaultResolverURL = "https://cloudflare-dns.com/dns-query"

const (
	maxDoH       = 64 << 10
	txtType      = 16
	SPFMissing   = "missing"
	SPFWeak      = "weak"
	MailOK       = "ok"
	DMARCNone    = "none" // a policy of "none" reports but does not protect
	DMARCMissing = "missing"
)

// Mail reads a domain's SPF and DMARC records over DNS-over-HTTPS, so the router cannot answer in the resolver's place.
type Mail struct {
	Opener      Opener
	ResolverURL func() string
}

// MailRecords is what the two records say, as flags: SPF is missing, weak or ok; DMARC is missing, none or ok.
type MailRecords struct{ SPF, DMARC string }

// Look asks for the two TXT records. Either question failing fails the whole look: half an answer is not "fine".
func (m Mail) Look(ctx context.Context, domain string) (MailRecords, error) {
	apex, err := m.txt(ctx, domain)
	if err != nil {
		return MailRecords{}, err
	}
	policy, err := m.txt(ctx, "_dmarc."+domain)
	if err != nil {
		return MailRecords{}, err
	}
	return MailRecords{SPF: judgeSPF(apex), DMARC: judgeDMARC(policy)}, nil
}

func (m Mail) txt(ctx context.Context, name string) ([]string, error) {
	base := DefaultResolverURL
	if m.ResolverURL != nil && m.ResolverURL() != "" {
		base = m.ResolverURL()
	}
	body, err := get(ctx, m.Opener, base+"?name="+url.QueryEscape(name)+"&type=TXT", "application/dns-json", nil, maxDoH)
	if err != nil {
		return nil, err
	}
	var answer struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return nil, &LookError{Reason: ReasonUnreadable}
	}
	// Status 0 is NOERROR and 3 is NXDOMAIN (the record does not exist): both are answers. Anything else is the resolver failing.
	if answer.Status != 0 && answer.Status != 3 {
		return nil, &LookError{Reason: ReasonDown}
	}
	var out []string
	for _, a := range answer.Answer {
		if a.Type == txtType {
			out = append(out, strings.ReplaceAll(strings.Trim(a.Data, `"`), `" "`, ""))
		}
	}
	return out, nil
}

// judgeSPF reads the domain's SPF record: none is missing; one that ends in "+all" or "?all" lets anyone send as the domain.
func judgeSPF(records []string) string {
	for _, r := range records {
		fields := strings.Fields(strings.ToLower(r))
		if len(fields) == 0 || fields[0] != "v=spf1" {
			continue
		}
		last := fields[len(fields)-1]
		if last == "+all" || last == "?all" || last == "all" {
			return SPFWeak
		}
		return MailOK
	}
	return SPFMissing
}

// judgeDMARC reads the _dmarc record: none is missing; p=none only reports.
func judgeDMARC(records []string) string {
	for _, r := range records {
		fields := strings.Split(strings.ToLower(strings.ReplaceAll(r, " ", "")), ";")
		if len(fields) == 0 || fields[0] != "v=dmarc1" {
			continue
		}
		for _, f := range fields[1:] {
			if f == "p=none" {
				return DMARCNone
			}
		}
		return MailOK
	}
	return DMARCMissing
}
