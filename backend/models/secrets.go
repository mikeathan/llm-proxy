package models

import "errors"

var ErrSecretNotFound = errors.New("secret not found")

type SecretData struct {
	Version      int                      `json:"version"`
	ProviderKeys map[string][]SecretEntry `json:"provider_keys,omitempty"`
}

type EncryptedSecretData struct {
	Version    int    `json:"version"`
	Ciphertext string `json:"ciphertext,omitempty"`
	Nonce      string `json:"nonce,omitempty"`
}

type SecretsStore interface {
	GetProviderKeys(provider string) []APIKeyItem
	SetProviderKeys(provider string, keys []APIKeyItem) error
	DeleteProviderKey(provider, keyID string) error
	MaskedProviderKeys(provider string) []APIKeyItem

	GetSecret(category, provider string) string
	SetSecret(category, provider, value string) error
	MaskedSecret(category, provider string) string

	GetResolvedProviderKey(provider, name string) (string, error)

	ResolveMaskedKey(provider, maskedKey string) (string, error)
}

type SecretEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}
