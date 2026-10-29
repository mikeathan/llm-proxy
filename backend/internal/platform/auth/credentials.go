package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

// Scope says what a script token may do.
type Scope string

const (
	// ScopeAdmin reaches the admin API, except the actions that need the passphrase (it cannot confirm one).
	ScopeAdmin Scope = "admin"
	// ScopeInference reaches only the model proxy (/v1).
	ScopeInference Scope = "inference"
)

// TokenPrefix starts every script token, so a pasted one is recognisable (secret scanners and the log redactor look for it).
const TokenPrefix = "lp_"

// Token is a named script credential. Only its SHA-256 is kept; the token itself is shown once, when it is made.
type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     Scope     `json:"scope"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// Session is one browser signed in. ID is the SHA-256 of the cookie value, never the value itself.
type Session struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeen      time.Time `json:"last_seen"`
	Address       string    `json:"address,omitempty"`
	Browser       string    `json:"browser,omitempty"`
	ElevatedUntil time.Time `json:"elevated_until,omitempty"`
}

// SetupCode lets the first passphrase be set from the UI by someone who can run a command on the server. It is stored as a
// hash, expires, is used once, and is cancelled after a few wrong guesses.
type SetupCode struct {
	Hash      string    `json:"hash"`
	ExpiresAt time.Time `json:"expires_at"`
	Attempts  int       `json:"attempts,omitempty"`
}

// Event is one line of the sign-in log shown on the Security page. It never holds a secret.
type Event struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Address string    `json:"address,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

const (
	tokenBytes     = 32
	sessionBytes   = 32
	idPrefixLength = 16
	// setupAlphabet leaves out 0, O, 1, I and L, which are read wrongly aloud and in some fonts.
	setupAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

var tokenEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// hashSecret is the stored form of a token, a session id or a setup code: a SHA-256, hex.
func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// shortID is the part of a hash shown in lists and used to name one entry to revoke.
func shortID(hash string) string {
	if len(hash) < idPrefixLength {
		return hash
	}
	return hash[:idPrefixLength]
}

func newToken() (plain, hash string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = TokenPrefix + strings.ToLower(tokenEncoding.EncodeToString(b))
	return plain, hashSecret(plain), nil
}

func newSessionID() (string, error) {
	b := make([]byte, sessionBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newSetupCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 0, 9)
	for i, v := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, setupAlphabet[int(v)%len(setupAlphabet)])
	}
	return string(out), nil
}

// normaliseSetupCode accepts the code in any case, with spaces, with or without the dash.
func normaliseSetupCode(s string) string {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
	if len(s) == 8 {
		return s[:4] + "-" + s[4:]
	}
	return s
}
