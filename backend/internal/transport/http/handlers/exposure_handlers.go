package handlers

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"llm-proxy/internal/platform/auth"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// exposureView is the Sign-in page's "Exposure" card: where the server listens, what protects it, and the items that
// need attention, each with what to do. It holds no secret.
type exposureView struct {
	Listening struct {
		Address string `json:"address"`
		// Network is true when other machines can connect (the bind is not a loopback address).
		Network bool `json:"network"`
	} `json:"listening"`
	Encrypted            bool `json:"encrypted"`
	InferenceKeyRequired bool `json:"inference_key_required"`
	Webhooks             struct {
		Inbound    int `json:"inbound"`
		WithSecret int `json:"with_secret"`
	} `json:"webhooks"`
	Sandbox   exposureSandbox     `json:"sandbox"`
	Attention []exposureAttention `json:"attention"`
}

type exposureSandbox struct {
	Enabled bool `json:"enabled"`
	// Filesystem is the OS file confinement agent children run under: a mechanism name, or "none".
	Filesystem string `json:"filesystem"`
}

type exposureAttention struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Fix  string `json:"fix"`
}

// tokenIdleLimit is how long a script token may go unused before the card asks whether it is still needed.
const tokenIdleLimit = 30 * 24 * time.Hour

// exposureToken is what the card needs of a script token: when it was made and last used. Never its name or secret.
type exposureToken struct {
	CreatedAt time.Time
	LastUsed  time.Time
}

// exposureInput is what the card is built from, gathered by the handler.
type exposureInput struct {
	Bind                 string
	ModelHost            string
	Tokens               []exposureToken
	Now                  time.Time
	Encrypted            bool
	InferenceKeyRequired bool
	Connectors           map[string]models.ConnectorConfig
	Sandbox              bool
	FilesystemMechanism  string
}

// isLoopbackBind reports whether a bind address ("host:port") accepts connections from this machine only. An empty
// host (":4001") and the wildcard addresses listen on every interface.
func isLoopbackBind(bind string) bool {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func buildExposure(in exposureInput) exposureView {
	var v exposureView
	v.Listening.Address = in.Bind
	v.Listening.Network = !isLoopbackBind(in.Bind)
	v.Encrypted, v.InferenceKeyRequired = in.Encrypted, in.InferenceKeyRequired
	v.Sandbox = exposureSandbox{Enabled: in.Sandbox, Filesystem: in.FilesystemMechanism}
	if v.Sandbox.Filesystem == "" {
		v.Sandbox.Filesystem = "none"
	}
	for _, c := range in.Connectors {
		if !c.Enabled || c.Settings["workspace_id"] == "" {
			continue
		}
		v.Webhooks.Inbound++
		if c.Settings[models.ConnectorSettingWebhookToken] != "" {
			v.Webhooks.WithSecret++
		}
	}
	v.Attention = exposureAttentionItems(v)
	v.Attention = append(v.Attention, programAttentionItems(in)...)
	if v.Attention == nil {
		v.Attention = []exposureAttention{} // an empty list, not null, in the JSON
	}
	return v
}

// exposureAttentionItems are the things worth acting on, most serious first. A closed-down install (loopback, signed in,
// sandbox on) has none.
func exposureAttentionItems(v exposureView) []exposureAttention {
	var out []exposureAttention
	network := v.Listening.Network
	if network && !v.Encrypted {
		out = append(out, exposureAttention{
			ID:   "unencrypted",
			Text: "This connection is not encrypted: your passphrase and session can be read by others on the network.",
			Fix:  "Put a TLS front end (Caddy, nginx, Tailscale) in front and list it in server.trusted_proxies, or set server.bind to 127.0.0.1. See the service setup guide.",
		})
	}
	if network && !v.InferenceKeyRequired {
		out = append(out, exposureAttention{
			ID:   "model_proxy_open",
			Text: "The model proxy (/v1) answers anyone who can reach it: no inference key exists.",
			Fix:  "Create a script token with the inference scope under Script access; once one exists, /v1 requires a key.",
		})
	}
	if v.Webhooks.WithSecret < v.Webhooks.Inbound {
		out = append(out, exposureAttention{
			ID:   "webhook_no_secret",
			Text: "A connector that receives messages has no webhook secret, so the server refuses its messages.",
			Fix:  "Open Settings, Communication, and create a secret for it, then register the webhook again.",
		})
	}
	if !v.Sandbox.Enabled {
		out = append(out, exposureAttention{
			ID:   "sandbox_off",
			Text: "The agent sandbox is off: the terminal tool and agent file access are not confined.",
			Fix:  "Turn the sandbox on under Settings, Safety.",
		})
	}
	return out
}

// programAttentionItems are the items about parts of this program other than its admin page: the model server it starts and the
// script tokens it holds.
func programAttentionItems(in exposureInput) []exposureAttention {
	var out []exposureAttention
	if in.ModelHost != "" && !isLoopbackBind(net.JoinHostPort(in.ModelHost, "0")) {
		out = append(out, exposureAttention{
			ID:   "model_server_open",
			Text: "The model server this program starts listens on every network address, so anyone on the network can use it directly. The /v1 inference key does not protect it.",
			Fix:  "Open Settings, System, and set the model server address to 127.0.0.1. The next model start listens on this computer only.",
		})
	}
	if n := idleTokens(in.Tokens, in.Now); n > 0 {
		out = append(out, exposureAttention{
			ID:   "token_unused",
			Text: idleTokensText(n),
			Fix:  "Revoke the tokens you no longer use under Script access.",
		})
	}
	return out
}

// idleTokensText says how many tokens are idle, in fixed words.
func idleTokensText(n int) string {
	subject := "1 script token has"
	if n != 1 {
		subject = fmt.Sprintf("%d script tokens have", n)
	}
	return subject + " not been used for 30 days. A token nobody uses is a key nobody watches."
}

// idleTokens counts the tokens made more than tokenIdleLimit ago and not used within it.
func idleTokens(tokens []exposureToken, now time.Time) int {
	n := 0
	for _, t := range tokens {
		last := t.LastUsed
		if last.IsZero() {
			last = t.CreatedAt
		}
		if now.Sub(last) >= tokenIdleLimit {
			n++
		}
	}
	return n
}

// ExposureSource is what the card reads from the running server.
type ExposureSource struct {
	Auth interface{ InferenceRequired() bool }
	Bind func() string
	// ModelHost is the address the model server is started on; Tokens the script tokens. Either may be nil.
	ModelHost      func() string
	Tokens         func() []auth.TokenView
	Registry       func() models.RegistryData
	Host           func() models.HostSettings
	TrustedProxies func() []string
}

// ExposureHandlers serves GET /admin/api/auth/exposure.
type ExposureHandlers struct{ src ExposureSource }

func NewExposureHandlers(src ExposureSource) *ExposureHandlers { return &ExposureHandlers{src: src} }

// Get answers with the exposure card. It is behind sign-in like the rest of the admin API: it describes the setup.
func (h *ExposureHandlers) Get(w http.ResponseWriter, r *http.Request) {
	host := h.src.Host()
	in := exposureInput{
		Bind:                 h.src.Bind(),
		Encrypted:            api.IsHTTPS(r, h.src.TrustedProxies),
		InferenceKeyRequired: h.src.Auth.InferenceRequired(),
		Connectors:           h.src.Registry().Communication.Connectors,
		Sandbox:              host.Sandboxing.Enabled,
		Now:                  time.Now(),
	}
	if h.src.ModelHost != nil {
		in.ModelHost = h.src.ModelHost()
	}
	if h.src.Tokens != nil {
		for _, t := range h.src.Tokens() {
			in.Tokens = append(in.Tokens, exposureToken{CreatedAt: t.CreatedAt, LastUsed: t.LastUsed})
		}
	}
	if host.Effective != nil {
		in.FilesystemMechanism = host.Effective.Filesystem.Mechanism
	}
	respondJSON(w, buildExposure(in))
}
