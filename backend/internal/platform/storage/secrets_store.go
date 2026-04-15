package storage

import (
	"fmt"
	"llm-proxy/models"
	"sync"
)

// SecretStore implements models.SecretsStore using a technical Store[models.SecretData].
// It handles high-level logic like masking, hydration, and credential resolution.
type SecretStore struct {
	store *Store[models.SecretData]
	mu    sync.RWMutex
}

func NewSecretStore(store *Store[models.SecretData]) *SecretStore {
	return &SecretStore{store: store}
}

// GetProviderKeys returns a copy of all API keys for the given provider.
func (b *SecretStore) GetProviderKeys(provider string) []models.APIKeyItem {
	data := b.store.Get()
	entries, ok := data.ProviderKeys[provider]
	if !ok {
		return []models.APIKeyItem{}
	}
	res := make([]models.APIKeyItem, len(entries))
	for i, e := range entries {
		res[i] = models.APIKeyItem{
			ID:   e.ID,
			Name: e.Name,
			Key:  e.Key,
		}
	}
	return res
}

// SetProviderKeys replaces the full key set for a provider.
func (b *SecretStore) SetProviderKeys(provider string, keys []models.APIKeyItem) error {
	return b.store.Update(func(data *models.SecretData) {
		if data.ProviderKeys == nil {
			data.ProviderKeys = make(map[string][]models.SecretEntry)
		}

		// Build lookup for hydration if needed
		existingByID := make(map[string]string)
		for _, e := range data.ProviderKeys[provider] {
			existingByID[e.ID] = e.Key
		}

		newEntries := make([]models.SecretEntry, len(keys))
		for i, k := range keys {
			keyVal := k.Key
			if IsMasked(keyVal) {
				if real, ok := existingByID[k.ID]; ok {
					keyVal = real
				}
			}
			newEntries[i] = models.SecretEntry{
				ID:   k.ID,
				Name: k.Name,
				Key:  keyVal,
			}
		}
		data.ProviderKeys[provider] = newEntries
	})
}

// DeleteProviderKey removes a single key by ID from a provider's key set.
func (b *SecretStore) DeleteProviderKey(provider, keyID string) error {
	return b.store.Update(func(data *models.SecretData) {
		entries, ok := data.ProviderKeys[provider]
		if !ok {
			return
		}
		filtered := entries[:0]
		for _, e := range entries {
			if e.ID != keyID {
				filtered = append(filtered, e)
			}
		}
		data.ProviderKeys[provider] = filtered
	})
}

// MaskedProviderKeys returns the provider keys with their secret values redacted.
func (b *SecretStore) MaskedProviderKeys(provider string) []models.APIKeyItem {
	keys := b.GetProviderKeys(provider)
	for i := range keys {
		keys[i].Key = MaskKey(keys[i].Key)
	}
	return keys
}

// GetSecret (Tool Secret) - for the bridge we map these to the "tools" provider group
func (b *SecretStore) GetSecret(category, provider string) string {
	data := b.store.Get()
	key := category + ":" + provider
	for _, entries := range data.ProviderKeys {
		for _, e := range entries {
			if e.ID == key {
				return e.Key
			}
		}
	}
	return ""
}

func (b *SecretStore) SetSecret(category, provider, value string) error {
	key := category + ":" + provider
	return b.store.Update(func(data *models.SecretData) {
		if data.ProviderKeys == nil {
			data.ProviderKeys = make(map[string][]models.SecretEntry)
		}
		// Try to update existing
		for p, entries := range data.ProviderKeys {
			for i, e := range entries {
				if e.ID == key {
					if IsMasked(value) {
						value = e.Key
					}
					data.ProviderKeys[p][i].Key = value
					return
				}
			}
		}
		// If not found, add to "tools" provider
		data.ProviderKeys["tools"] = append(data.ProviderKeys["tools"], models.SecretEntry{
			ID:   key,
			Name: key,
			Key:  value,
		})
	})
}

func (b *SecretStore) MaskedSecret(category, provider string) string {
	val := b.GetSecret(category, provider)
	if val == "" {
		return ""
	}
	return MaskKey(val)
}

func (b *SecretStore) GetResolvedProviderKey(provider, name string) (string, error) {
	keys := b.GetProviderKeys(provider)
	if len(keys) == 0 {
		return "", fmt.Errorf("provider %q not found", provider)
	}

	if name != "" {
		for _, k := range keys {
			if k.Name == name || k.ID == name {
				return k.Key, nil
			}
		}
		return "", fmt.Errorf("key %q not found for provider %q", name, provider)
	}

	return keys[0].Key, nil
}

func (b *SecretStore) ResolveMaskedKey(provider, maskedKey string) (string, error) {
	keys := b.GetProviderKeys(provider)
	if !IsMasked(maskedKey) {
		return "", fmt.Errorf("provided key is not masked")
	}

	for _, k := range keys {
		if MaskKey(k.Key) == maskedKey {
			return k.Key, nil
		}
	}

	return "", fmt.Errorf("no key matching mask found for %q", provider)
}
