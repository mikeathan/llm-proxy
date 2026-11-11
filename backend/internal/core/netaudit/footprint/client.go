package footprint

import (
	"context"
	"errors"
	"io"
	"net/http"

	"llm-proxy/internal/core/tools"
)

// Reasons a look could not be made. They are fixed codes with fixed sentences (ReasonText): what a service or the network said
// is never copied into a result, so an outside service cannot put its own words on the screen.
const (
	ReasonOff        = "off"          // the network settings do not allow the request
	ReasonDown       = "service_down" // the service did not answer, or answered with an error
	ReasonUnreadable = "unreadable"   // the answer was not what the service documents, or too large to read
	ReasonNoRegistry = "no_registry"  // no registry service is listed for the domain's ending
	ReasonNoExpiry   = "no_expiry"    // the registry gave no expiry date
	ReasonNoKey      = "no_key"       // a breach key is needed and none is saved
	ReasonKeyRefused = "key_refused"  // the service did not accept the key, or the key's plan does not include this search
	ReasonRateLimit  = "rate_limited" // the service asked to be called less often
	ReasonOutOfDate  = "out_of_date"  // the last successful look is older than its schedule allows
	ReasonSwitchOff  = "switch_off"   // the operator has not turned this service on
)

var reasonText = map[string]string{
	ReasonOff:        "Network access is switched off in the settings.",
	ReasonDown:       "The service did not answer. It will be tried again at the next look.",
	ReasonUnreadable: "The service's answer could not be read.",
	ReasonNoRegistry: "No registry service that can be asked over a secure connection is listed for this domain's ending.",
	ReasonNoExpiry:   "The registry did not give an expiry date.",
	ReasonNoKey:      "A breach-service key is needed and none is saved.",
	ReasonKeyRefused: "The breach service did not accept the key, or the key's plan does not include this search.",
	ReasonRateLimit:  "The service asked to be called less often. It will be tried again at the next look.",
	ReasonOutOfDate:  "The last look is older than its schedule allows, so this is not known.",
	ReasonSwitchOff:  "This service is switched off.",
}

// ReasonText is the fixed sentence for a reason code.
func ReasonText(reason string) string {
	if t, ok := reasonText[reason]; ok {
		return t
	}
	return reasonText[ReasonDown]
}

// LookError is a look that could not be made, with its reason code.
type LookError struct{ Reason string }

func (e *LookError) Error() string { return ReasonText(e.Reason) }

// ReasonOf is the reason code of an error from a look.
func ReasonOf(err error) string {
	var le *LookError
	if errors.As(err, &le) {
		return le.Reason
	}
	return ReasonDown
}

// Opener makes a guarded GET with headers: *tools.NetworkTools.
type Opener interface {
	OpenWith(ctx context.Context, target, accept string, headers map[string]string) (*http.Response, error)
}

// get makes one guarded GET and reads at most max bytes; an answer longer than that is "unreadable", never a cut-off parse.
func get(ctx context.Context, o Opener, target, accept string, headers map[string]string, max int64) ([]byte, error) {
	resp, err := o.OpenWith(ctx, target, accept, headers)
	if err != nil {
		return nil, &LookError{Reason: reasonOfRequestError(err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, &LookError{Reason: ReasonDown}
	}
	if int64(len(body)) > max {
		return nil, &LookError{Reason: ReasonUnreadable}
	}
	return body, nil
}

func reasonOfRequestError(err error) string {
	var status *tools.StatusError
	switch {
	case errors.Is(err, tools.ErrNetworkDisabled), errors.Is(err, tools.ErrInternetBlocked):
		return ReasonOff
	case errors.As(err, &status) && (status.Code == http.StatusUnauthorized || status.Code == http.StatusForbidden):
		return ReasonKeyRefused
	case errors.As(err, &status) && status.Code == http.StatusTooManyRequests:
		return ReasonRateLimit
	}
	return ReasonDown
}
