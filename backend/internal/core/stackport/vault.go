package stackport

import (
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"regexp"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/secretcrypto"
)

const (
	msgBadPurpose = "the purpose is not a short lower-case token"
	msgBadValue   = "the value is empty or too large"
	msgNoKey      = "the core has no key yet"
	msgNoOpen     = "the sealed value does not open"
	vaultInfo     = "llm-proxy/stack-vault/"
)

var purposeToken = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

var _ stacksdk.Vault = Vault{}

// Vault is the SDK's Vault port over the core's master key (Constitution III.6). A subkey is derived for each purpose (HKDF-SHA256), so a tag
// of one purpose cannot be matched to a tag of another and the stack never sees a key. A tag is HMAC-SHA256; a sealed value is AES-256-GCM through
// secretcrypto, the same envelope the secrets file uses.
type Vault struct {
	// Key returns the 32-byte master key; an error or any other length makes every call CodeUnavailable.
	Key func() ([]byte, error)
}

func (v Vault) subkey(purpose string) ([]byte, error) {
	if !purposeToken.MatchString(purpose) {
		return nil, stacksdk.Errorf(stacksdk.CodeInvalid, msgBadPurpose)
	}
	if v.Key == nil {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, msgNoKey)
	}
	master, err := v.Key()
	if err != nil || len(master) != 32 {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, msgNoKey)
	}
	sub, err := hkdf.Key(sha256.New, master, nil, vaultInfo+purpose, 32)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, msgNoKey)
	}
	return sub, nil
}

func checkValue(b []byte) error {
	if len(b) == 0 || len(b) > stacksdk.MaxVaultBytes {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgBadValue)
	}
	return nil
}

// Tag returns HMAC-SHA256 of value under the purpose's subkey.
func (v Vault) Tag(_ context.Context, purpose string, value []byte) ([]byte, error) {
	if err := checkValue(value); err != nil {
		return nil, err
	}
	key, err := v.subkey(purpose)
	if err != nil {
		return nil, err
	}
	m := hmac.New(sha256.New, key)
	m.Write(value)
	return m.Sum(nil), nil
}

// Seal encrypts value; the result is "ciphertext.nonce" in base64, different on every call.
func (v Vault) Seal(_ context.Context, purpose string, value []byte) ([]byte, error) {
	if err := checkValue(value); err != nil {
		return nil, err
	}
	key, err := v.subkey(purpose)
	if err != nil {
		return nil, err
	}
	ct, nonce, err := secretcrypto.EncryptAES(key, value)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, msgNoOpen)
	}
	return []byte(ct + "." + nonce), nil
}

// Open returns what Seal sealed for the same purpose, or CodeDenied.
func (v Vault) Open(_ context.Context, purpose string, sealed []byte) ([]byte, error) {
	key, err := v.subkey(purpose)
	if err != nil {
		return nil, err
	}
	ct, nonce, ok := strings.Cut(string(sealed), ".")
	if !ok || len(sealed) > 2*stacksdk.MaxVaultBytes+64 {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, msgNoOpen)
	}
	plain, err := secretcrypto.DecryptAES(key, ct, nonce)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, msgNoOpen)
	}
	return plain, nil
}
