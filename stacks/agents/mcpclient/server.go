// Package mcpclient is a general MCP client for tools (ADR-0069): it connects to the servers the owner listed over Streamable HTTP, falling back to the
// old HTTP+SSE transport as the specification asks, keeps the tools each one offers in a cache, and calls one by name. It carries no sign-in of its own
// beyond a bearer token or one header, which it is handed already opened, and it dials only through the HTTP client it is given (stacksdk.HTTP).
package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Trust says whether the owner approves each call of a server's tools.
type Trust string

// The two trust settings. Ask is the default: an unknown value reads as Ask, never as Allow.
const (
	TrustAsk   Trust = "ask"
	TrustAllow Trust = "allow"
)

// AuthKind is how a server is signed in to. OAuth is not offered yet; a new kind is added here and in Auth.Validate.
type AuthKind string

// The ways to sign in.
const (
	AuthNone   AuthKind = "none"
	AuthBearer AuthKind = "bearer" // Authorization: Bearer <value>
	AuthHeader AuthKind = "header" // one named header with the value
)

// Auth is a server's sign-in. Sealed is the value encrypted with the vault (the stack never stores it in the clear); it is never sent to the screen.
type Auth struct {
	Kind   AuthKind `json:"kind"`
	Header string   `json:"header,omitempty"`
	Sealed string   `json:"sealed,omitempty"`
}

// Server is one MCP server the owner listed, as the stack's settings keep it.
type Server struct {
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Enabled       bool     `json:"enabled"`
	Trust         Trust    `json:"trust,omitempty"`
	Auth          Auth     `json:"auth,omitzero"`
	CACert        string   `json:"ca_cert,omitempty"`
	DisabledTools []string `json:"disabled_tools,omitempty"`
}

// The bounds on what a server entry may hold.
const (
	maxNameLen        = 24
	maxURLLen         = 2048
	maxPathLen        = 4096
	maxDisabledTools  = 256
	maxToolNameLen    = 128
	maxHeaderNameLen  = 64
	toolNamePrefix    = "mcp__"
	toolNameSeparator = "__"
	hashLen           = 6
)

var (
	namePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
	headerPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	// headersOwned are the headers the client sets itself; a server entry may not override one.
	headersOwned = []string{"host", "content-length", "content-type", "accept", "mcp-session-id", "mcp-protocol-version", "transfer-encoding", "connection", "upgrade", "cookie", "origin", "last-event-id"}
)

// ErrInvalid marks a server entry that cannot be used; its message says what to change.
var ErrInvalid = errors.New("invalid MCP server")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Normalize returns the entry with the defaults filled in: Ask when no trust is set (or an unknown one), no sign-in when none is set.
func (s Server) Normalize() Server {
	if s.Trust != TrustAllow {
		s.Trust = TrustAsk
	}
	if s.Auth.Kind == "" {
		s.Auth.Kind = AuthNone
	}
	return s
}

// Validate refuses an entry that cannot be used, in words that name the field to change.
func (s Server) Validate() error {
	if !namePattern.MatchString(s.Name) {
		return invalid("the name must be lower-case letters, digits and hyphens, start with a letter and be at most %d characters", maxNameLen)
	}
	if err := validateURL(s.URL); err != nil {
		return err
	}
	if s.Trust != "" && s.Trust != TrustAsk && s.Trust != TrustAllow {
		return invalid("trust must be %q or %q", TrustAsk, TrustAllow)
	}
	if err := s.Auth.Validate(); err != nil {
		return err
	}
	return s.validateLimits()
}

// validateLimits holds the certificate authority path and the list of tools switched off to their bounds.
func (s Server) validateLimits() error {
	if len(s.CACert) > maxPathLen || (s.CACert != "" && !strings.HasPrefix(s.CACert, "/")) {
		return invalid("the certificate authority file must be an absolute path")
	}
	if len(s.DisabledTools) > maxDisabledTools {
		return invalid("at most %d tools can be switched off", maxDisabledTools)
	}
	for _, t := range s.DisabledTools {
		if t == "" || len(t) > maxToolNameLen {
			return invalid("a switched-off tool needs a name of at most %d characters", maxToolNameLen)
		}
	}
	return nil
}

func validateURL(raw string) error {
	if raw == "" || len(raw) > maxURLLen {
		return invalid("the address is required and at most %d characters", maxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return invalid("the address must be an http or https URL with a host")
	}
	if u.User != nil {
		return invalid("the address must not hold a user name or password; use the sign-in setting")
	}
	if u.Fragment != "" {
		return invalid("the address must not hold a fragment")
	}
	return nil
}

// Validate checks the sign-in kind and, for a named header, that the name is one a client may set.
func (a Auth) Validate() error {
	switch a.Kind {
	case "", AuthNone, AuthBearer:
		if a.Header != "" {
			return invalid("a header name only goes with the %q sign-in", AuthHeader)
		}
	case AuthHeader:
		if !headerPattern.MatchString(a.Header) || slices.Contains(headersOwned, strings.ToLower(a.Header)) {
			return invalid("the header name must be letters, digits and hyphens, at most %d characters, and not one the client sets itself", maxHeaderNameLen)
		}
	default:
		return invalid("the sign-in must be %q, %q or %q", AuthNone, AuthBearer, AuthHeader)
	}
	return nil
}

// Headers returns the request headers for an opened secret. An empty secret on a kind that needs one is an error: a half-set sign-in never goes out as none.
func (a Auth) Headers(secret string) (map[string]string, error) {
	switch a.Kind {
	case "", AuthNone:
		return nil, nil
	case AuthBearer:
		if secret == "" {
			return nil, invalid("the bearer token is missing")
		}
		return map[string]string{"Authorization": "Bearer " + secret}, nil
	case AuthHeader:
		if secret == "" {
			return nil, invalid("the header value is missing")
		}
		return map[string]string{a.Header: secret}, nil
	}
	return nil, invalid("the sign-in kind is unknown")
}

// NeedsSecret reports whether the sign-in kind carries a value.
func (a Auth) NeedsSecret() bool { return a.Kind == AuthBearer || a.Kind == AuthHeader }

// sameConnection reports whether two entries open the same connection: when it is true a change between them needs no reconnect.
func (s Server) sameConnection(o Server) bool {
	return s.URL == o.URL && s.Auth == o.Auth && s.CACert == o.CACert && s.Enabled == o.Enabled
}

// ToolName is the name the model sees for one tool of one server: mcp__<server>__<tool>, at most limit characters. A tool name with characters the
// model's interface refuses, or one that makes the name too long, is cleaned and gets a short hash of the original so two different tools never
// share a name.
func ToolName(server, tool string, limit int) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		}
		return '_'
	}, tool)
	prefix := toolNamePrefix + server + toolNameSeparator
	name := prefix + clean
	if clean == tool && len(name) <= limit {
		return name
	}
	sum := sha256.Sum256([]byte(tool))
	suffix := "_" + hex.EncodeToString(sum[:])[:hashLen]
	if room := limit - len(prefix) - len(suffix); len(clean) > room {
		clean = clean[:max(room, 0)]
	}
	return prefix + clean + suffix
}
