package footprint

import (
	"context"
	"crypto/sha1" //nolint:gosec // the breach service's own k-anonymity scheme is defined over SHA-1; it is a lookup key, not a protection
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// BreachURL is the breach service's k-anonymity search for email addresses. It needs the operator's own key, and the key's plan
// must include this search (the service documents it for its Pro plans). Only the first six characters of the address's SHA-1
// hash are sent; the service answers with every matching suffix, and all that do not match the address are dropped at once, as
// its terms require.
const BreachURL = "https://haveibeenpwned.com/api/v3/breachedaccount/range/"

const (
	maxBreachAnswer = 2 << 20
	prefixLen       = 6
	// MaxBreachNames bounds the names kept per address.
	MaxBreachNames = 50
	userAgent      = "llm-proxy-footprint"
)

var breachNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._\-]{0,39}$`)

// Breaches looks an address up. Key is read on every look, so a changed key takes effect at once.
type Breaches struct {
	Opener Opener
	Key    func() string
}

// Look returns the names of the breaches the address is in (none: not found in any). A name that is not a short plain word
// is not kept, but the address still counts as found in that breach, so Found can exceed len(names).
func (b Breaches) Look(ctx context.Context, address string) (names []string, found int, err error) {
	key := ""
	if b.Key != nil {
		key = strings.TrimSpace(b.Key())
	}
	if key == "" {
		return nil, 0, &LookError{Reason: ReasonNoKey}
	}
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(address)))) //nolint:gosec // see BreachURL
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	body, err := get(ctx, b.Opener, BreachURL+full[:prefixLen], "application/json", map[string]string{"hibp-api-key": key, "User-Agent": userAgent}, maxBreachAnswer)
	if err != nil {
		return nil, 0, err
	}
	var rows []struct {
		Suffix   string   `json:"hashSuffix"`
		Websites []string `json:"websites"`
	}
	if json.Unmarshal(body, &rows) != nil {
		return nil, 0, &LookError{Reason: ReasonUnreadable}
	}
	for _, r := range rows {
		if !strings.EqualFold(r.Suffix, full[prefixLen:]) {
			continue // another address in the same range: discarded, never stored
		}
		for _, w := range r.Websites {
			found++
			if breachNameRE.MatchString(w) && len(names) < MaxBreachNames {
				names = append(names, w)
			}
		}
	}
	return names, found, nil
}
