// Package auth is the operator's sign-in: a passphrase, named script tokens and persisted sessions, kept as hashes in one
// file under the config root. Plan: docs/PLANS/cross-cutting/admin-api-authentication-design.md.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	saltBytes = 16
	keyBytes  = 32
	// maxConcurrentHashes bounds how many passphrase checks run at once: each needs tens of megabytes, so a flood of
	// login attempts must queue, not exhaust memory.
	maxConcurrentHashes = 2
	// MinPassphraseLength is the shortest passphrase accepted. There are no composition rules: length is what counts.
	MinPassphraseLength = 12
)

// Params are the argon2id cost settings. They are stored inside each hash, so they can be raised later without
// invalidating a passphrase that is already set.
type Params struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultParams are at or above the OWASP minimum for argon2id; the memory and time are meant to take roughly 150 ms on
// the server this runs on.
func DefaultParams() Params { return Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 2} }

var errMalformedHash = errors.New("the stored passphrase hash is malformed")

// Hasher hashes and checks passphrases with argon2id.
type Hasher struct {
	params Params
	sem    chan struct{}
}

func NewHasher(p Params) *Hasher {
	return &Hasher{params: p, sem: make(chan struct{}, maxConcurrentHashes)}
}

// Hash returns the PHC-style encoding of the passphrase: $argon2id$v=19$m=…,t=…,p=…$salt$hash.
func (h *Hasher) Hash(passphrase string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to make a salt: %w", err)
	}
	key := argon2.IDKey([]byte(passphrase), salt, h.params.Time, h.params.MemoryKiB, h.params.Threads, keyBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.params.MemoryKiB, h.params.Time, h.params.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify checks a passphrase against an encoding, using the cost stored in the encoding. It waits for a free slot, or
// gives up when ctx ends, so a flood of attempts queues instead of exhausting memory.
func (h *Hasher) Verify(ctx context.Context, encoded, passphrase string) (bool, error) {
	p, salt, want, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(passphrase), salt, p.Time, p.MemoryKiB, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func parseHash(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "", argon2id, v=19, m=..,t=..,p=.., salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Params{}, nil, nil, errMalformedHash
	}
	var p Params
	var threads uint
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &threads); err != nil || threads == 0 || threads > 255 {
		return Params{}, nil, nil, errMalformedHash
	}
	p.Threads = uint8(threads)
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	key, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(salt) == 0 || len(key) == 0 {
		return Params{}, nil, nil, errMalformedHash
	}
	return p, salt, key, nil
}

// commonPassphrases are refused: the most guessed ones that are long enough to pass the length rule. This is a short
// list on purpose; it stops the obvious, and length does the rest.
var commonPassphrases = map[string]bool{
	"password1234": true, "password12345": true, "123456789012": true, "1234567890123": true, "qwertyuiop12": true,
	"qwertyuiop123": true, "iloveyou1234": true, "administrator": true, "letmein12345": true, "welcome12345": true,
	"changeme1234": true, "passwordpassword": true, "123123123123": true, "abcdefghijkl": true, "000000000000": true,
	"1q2w3e4r5t6y": true, "qazwsxedcrfv": true, "trustno1trustno1": true, "adminadmin12": true, "p@ssw0rd1234": true,
}

// WeakError is a passphrase that was refused; its message is safe to show to the person who typed it.
type WeakError struct{ Reason string }

func (e *WeakError) Error() string { return e.Reason }

// CheckStrength refuses a passphrase that is too short, blank, one repeated character, or one of the commonest.
func CheckStrength(p string) error {
	switch {
	case len([]rune(p)) < MinPassphraseLength:
		return &WeakError{fmt.Sprintf("the passphrase must be at least %d characters; a few words is fine", MinPassphraseLength)}
	case strings.TrimFunc(p, unicode.IsSpace) == "":
		return &WeakError{"the passphrase cannot be blank"}
	case isOneCharacter(p):
		return &WeakError{"the passphrase cannot be one character repeated"}
	case commonPassphrases[strings.ToLower(p)]:
		return &WeakError{"that passphrase is too common; choose a few unrelated words"}
	}
	return nil
}

func isOneCharacter(p string) bool {
	r := []rune(p)
	for _, c := range r[1:] {
		if c != r[0] {
			return false
		}
	}
	return true
}
