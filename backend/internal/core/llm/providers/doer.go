// doer.go — the dedicated provider infrastructure HTTP client abstraction.
//
// Constitution I.1 forbids http.DefaultClient / http.Get.  Provider
// infrastructure (catalogue listing, /slots probes, connection tests) is a
// separate traffic class from agent tools: it uses this injected HTTPDoer,
// backed by the shared platform transport (network.SharedTransport) with a 45s
// overall timeout, and never inherits agent-tool LAN/internet guardrails (C1).
// The small consumer-defined interface keeps providers unit-testable with a stub
// and avoids a lateral dependency on internal/core/tools.
package providers

import (
	"net/http"
	"time"

	"llm-proxy/internal/platform/network"
)

// HTTPDoer is the minimal interface providers need to make HTTP calls.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultProviderDoer returns the dedicated pooled provider client (45s
// overall timeout) used when bootstrap has not injected one — keeping provider
// calls off http.DefaultClient and off agent-tool guardrails.
func defaultProviderDoer() HTTPDoer {
	return &http.Client{Transport: network.SharedTransport, Timeout: providerRequestTimeout}
}

// providerRequestTimeout bounds an individual provider infrastructure request
// (catalogue listing, /slots probe, connection test).
const providerRequestTimeout = 45 * time.Second

// slotsProbeTimeout bounds the /slots (or /v1/props) serving-context probe for
// local endpoints (S2) — a dead local server must not hang the listing.
const slotsProbeTimeout = 5 * time.Second
