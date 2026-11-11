package footprint

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
)

// CertLogURL is the public certificate-transparency search asked about a domain. It is free, needs no key, is often slow and is
// sometimes down: a look that fails reads "could not look", never "no new certificates".
const CertLogURL = "https://crt.sh/"

const (
	maxCertAnswer = 4 << 20
	// MaxCertIDs bounds the certificate ids kept per domain, newest first.
	MaxCertIDs = 500
)

// CertLog asks the log which certificates exist for a domain and its subdomains.
type CertLog struct{ Opener Opener }

// Certs returns the ids of the certificates the log holds for the domain and its subdomains, newest first and bounded. Only the
// ids are read: the names in a certificate are text the issuer chose and are not kept.
func (c CertLog) Certs(ctx context.Context, domain string) ([]int64, error) {
	body, err := get(ctx, c.Opener, CertLogURL+"?q="+url.QueryEscape("%."+domain)+"&output=json", "application/json", nil, maxCertAnswer)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(body, &rows) != nil {
		return nil, &LookError{Reason: ReasonUnreadable}
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.ID > 0 {
			ids = append(ids, r.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	if len(ids) > MaxCertIDs {
		ids = ids[:MaxCertIDs]
	}
	return ids, nil
}

// NewCerts counts the ids not seen before. The first look (no ids seen) is the baseline: it reports none, so a domain with
// a hundred certificates does not start with a hundred new ones.
func NewCerts(seen, now []int64) int {
	if len(seen) == 0 {
		return 0
	}
	known := make(map[int64]bool, len(seen))
	for _, id := range seen {
		known[id] = true
	}
	n := 0
	for _, id := range now {
		if !known[id] {
			n++
		}
	}
	return n
}
