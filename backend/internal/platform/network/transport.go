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
	"crypto/tls"
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

// newLLMChatTransport builds the pooled transport used by the LLM chat/stream
// client (proxy.LLMClient). HTTP/2 is deliberately disabled: several
// OpenAI-compatible gateways — notably NVIDIA's integrate.api.nvidia.com —
// close connections mid-framing on their HTTP/2 chat path (curl reports
// "Error in the HTTP2 framing layer"; Go's http.Client surfaces "unexpected
// EOF" on the POST) while their HTTP/1.1 path is stable. The official OpenAI
// Go SDK disables HTTP/2 for the same reason. HTTP/1.1 has no functional
// downside for chat completions / SSE.
func newLLMChatTransport(responseHeaderTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// ForceAttemptHTTP2=false + a non-nil empty TLSNextProto is the
		// canonical way to keep a custom transport on HTTP/1.1: Go only
		// auto-configures HTTP/2 when TLSNextProto is nil.
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
		ResponseHeaderTimeout: responseHeaderTimeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
}

// LLMChatTransport is the pooled HTTP/1.1-only transport for LLM chat/stream
// calls to local llama.cpp servers. The 10-minute response-header timeout
// accommodates long local prefill on reasoning models.
var LLMChatTransport = newLLMChatTransport(10 * time.Minute)

// CloudLLMChatTransport is the pooled HTTP/1.1-only transport for LLM
// chat/stream calls to cloud OpenAI-compatible gateways (NVIDIA, OpenAI,
// OpenRouter, ...). Its response-header timeout (45s) is shorter than the
// local transport's: NVIDIA's gateway holds a saturated free-tier request for
// ~60s and then drops the connection (client sees "unexpected EOF"); a 45s
// client-side bound converts that into a clean "timeout" classification and
// lets the retry fire before the server-side drop. Tunable here only.
var CloudLLMChatTransport = newLLMChatTransport(45 * time.Second)
