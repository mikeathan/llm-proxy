package models

import "errors"

var ErrSecretNotFound = errors.New("secret not found")

// SecretData represents the encrypted/isolated vault (Tier 2: secrets.json)
type SecretData struct {
	Version      int                      `json:"version"`
	ProviderKeys map[string][]SecretEntry `json:"provider_keys"`
}

// SecretsStore defines the behavior for managing credentials and sensitive data.
type SecretsStore interface {
	// Provider API keys (LLM Providers)
	GetProviderKeys(provider string) []APIKeyItem
	SetProviderKeys(provider string, keys []APIKeyItem) error
	DeleteProviderKey(provider, keyID string) error
	MaskedProviderKeys(provider string) []APIKeyItem

	// Tool Secrets (Search, Communication, etc.)
	GetSecret(category, provider string) string
	SetSecret(category, provider, value string) error
	MaskedSecret(category, provider string) string

	// GetResolvedProviderKey looks up a key by name or ID.
	GetResolvedProviderKey(provider, name string) (string, error)

	// ResolveMaskedKey finds a real key that matches the provided mask string.
	ResolveMaskedKey(provider, maskedKey string) (string, error)
}

type SecretEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}
