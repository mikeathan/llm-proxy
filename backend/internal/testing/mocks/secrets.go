package mocks

import (
	"llm-proxy/internal/platform/secrets"
	"llm-proxy/models"
)

type MockSecretsStore struct {
	GetProviderKeysFunc      func(provider string) []models.APIKeyItem
	SetProviderKeysFunc      func(provider string, keys []models.APIKeyItem) error
	DeleteProviderKeyFunc    func(provider, keyID string) error
	MaskedProviderKeysFunc   func(provider string) []models.APIKeyItem
	GetSecretFunc            func(category, provider string) string
	SetSecretFunc            func(category, provider, value string) error
	MaskedSecretFunc         func(category, provider string) string
	GetResolvedProviderKeyFunc func(provider, name string) (string, error)
	ResolveMaskedKeyFunc func(provider, maskedKey string) (string, error)
}

func (m *MockSecretsStore) GetProviderKeys(provider string) []models.APIKeyItem {
	if m.GetProviderKeysFunc != nil {
		return m.GetProviderKeysFunc(provider)
	}
	return nil
}

func (m *MockSecretsStore) SetProviderKeys(provider string, keys []models.APIKeyItem) error {
	if m.SetProviderKeysFunc != nil {
		return m.SetProviderKeysFunc(provider, keys)
	}
	return nil
}

func (m *MockSecretsStore) DeleteProviderKey(provider, keyID string) error {
	if m.DeleteProviderKeyFunc != nil {
		return m.DeleteProviderKeyFunc(provider, keyID)
	}
	return nil
}

func (m *MockSecretsStore) MaskedProviderKeys(provider string) []models.APIKeyItem {
	if m.MaskedProviderKeysFunc != nil {
		return m.MaskedProviderKeysFunc(provider)
	}
	return nil
}

func (m *MockSecretsStore) GetSecret(category, provider string) string {
	if m.GetSecretFunc != nil {
		return m.GetSecretFunc(category, provider)
	}
	return ""
}

func (m *MockSecretsStore) SetSecret(category, provider, value string) error {
	if m.SetSecretFunc != nil {
		return m.SetSecretFunc(category, provider, value)
	}
	return nil
}

func (m *MockSecretsStore) MaskedSecret(category, provider string) string {
	if m.MaskedSecretFunc != nil {
		return m.MaskedSecretFunc(category, provider)
	}
	return ""
}

func (m *MockSecretsStore) GetResolvedProviderKey(provider, name string) (string, error) {
	if m.GetResolvedProviderKeyFunc != nil {
		return m.GetResolvedProviderKeyFunc(provider, name)
	}
	return "", nil
}

func (m *MockSecretsStore) ResolveMaskedKey(provider, maskedKey string) (string, error) {
	if m.ResolveMaskedKeyFunc != nil {
		return m.ResolveMaskedKeyFunc(provider, maskedKey)
	}
	return "", nil
}

// Ensure interface compliance
var _ secrets.Store = (*MockSecretsStore)(nil)
