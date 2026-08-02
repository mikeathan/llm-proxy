// transport.go — the shared pooled HTTP transport for all outbound upstream
// traffic (LLM chat/stream proxying and provider infrastructure).
//
// This is the single source of truth for connection pooling.  Both the proxy
// chat client and the provider catalogue/slots/connection-test clients reuse it
// (Constitution I.1: never http.DefaultClient / http.Get).  Living in this leaf
// package keeps both `internal/core/proxy` and
// `internal/core/llm/providers` able to reference it without an import cycle.
package network

import (
	"net/http"
	"time"
)

// SharedTransport is a pooled transport shared by all outbound HTTP clients to
// ensure connection reuse and prevent socket exhaustion.  It is read-only after
// package init; consumers wrap it in their own *http.Client with a per-call
// timeout.
var SharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 10 * time.Minute,
	IdleConnTimeout:       90 * time.Second,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
}
