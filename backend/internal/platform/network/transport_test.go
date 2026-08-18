package network

import (
	"net/http"
	"testing"
	"time"
)

// TestLLMChatTransportForcesHTTP1 verifies the LLM chat transports never
// negotiate HTTP/2. NVIDIA's integrate.api.nvidia.com has a broken HTTP/2 path
// for chat completions (curl: "Error in the HTTP2 framing layer"; Go:
// "unexpected EOF" on POST) while its HTTP/1.1 path is stable, so the chat
// client must stay on HTTP/1.1. The canonical way to keep a custom transport
// on HTTP/1.1 is a non-nil empty TLSNextProto plus ForceAttemptHTTP2=false.
// SharedTransport is deliberately excluded: it serves provider infrastructure
// (catalogue listing, connection tests) where HTTP/2 is fine.
func TestLLMChatTransportForcesHTTP1(t *testing.T) {
	transports := map[string]*http.Transport{
		"local": LLMChatTransport,
		"cloud": CloudLLMChatTransport,
	}
	for name, tr := range transports {
		t.Run(name, func(t *testing.T) {
			if tr.ForceAttemptHTTP2 {
				t.Errorf("%s: ForceAttemptHTTP2 must be false", name)
			}
			if tr.TLSNextProto == nil {
				t.Errorf("%s: TLSNextProto must be non-nil (empty map disables HTTP/2)", name)
			}
			if len(tr.TLSNextProto) != 0 {
				t.Errorf("%s: TLSNextProto must be empty, got %d entries", name, len(tr.TLSNextProto))
			}
		})
	}
}

// TestCloudLLMChatTransportShorterHeaderTimeout locks the cloud transport's
// response-header timeout below the local one: NVIDIA's free-tier gateway holds
// a saturated request ~60s then drops the connection (unexpected EOF), so the
// cloud client bounds the wait at 45s to classify the failure as a clean
// client-side timeout and let the retry fire sooner.
func TestCloudLLMChatTransportShorterHeaderTimeout(t *testing.T) {
	if CloudLLMChatTransport.ResponseHeaderTimeout >= LLMChatTransport.ResponseHeaderTimeout {
		t.Errorf("cloud header timeout (%v) must be shorter than local (%v)",
			CloudLLMChatTransport.ResponseHeaderTimeout, LLMChatTransport.ResponseHeaderTimeout)
	}
	if CloudLLMChatTransport.ResponseHeaderTimeout != 45*time.Second {
		t.Errorf("cloud header timeout = %v, want 45s", CloudLLMChatTransport.ResponseHeaderTimeout)
	}
}
