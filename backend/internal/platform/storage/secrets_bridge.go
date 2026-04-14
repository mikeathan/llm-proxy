package storage

import (
	"fmt"
	"llm-proxy/models"
	"llm-proxy/internal/platform/secrets"
	"sync"
)

// SecretsBridge wraps the new Store[SecretData] into the legacy secrets.Store interface.
type SecretsBridge struct {
	store *Store[SecretData]
	mu    sync.RWMutex
}

func NewSecretsBridge(store *Store[SecretData]) *SecretsBridge {
	return &SecretsBridge{store: store}
}

// GetProviderKeys returns a copy of all API keys for the given provider.
func (b *SecretsBridge) GetProviderKeys(provider string) []models.APIKeyItem {
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
func (b *SecretsBridge) SetProviderKeys(provider string, keys []models.APIKeyItem) error {
	return b.store.Update(func(data *SecretData) {
		if data.ProviderKeys == nil {
			data.ProviderKeys = make(map[string][]SecretEntry)
		}
		
		// Build lookup for hydration if needed
		existingByID := make(map[string]string)
		for _, e := range data.ProviderKeys[provider] {
			existingByID[e.ID] = e.Key
		}

		newEntries := make([]SecretEntry, len(keys))
		for i, k := range keys {
			keyVal := k.Key
			if secrets.IsMasked(keyVal) {
				if real, ok := existingByID[k.ID]; ok {
					keyVal = real
				}
			}
			newEntries[i] = SecretEntry{
				ID:   k.ID,
				Name: k.Name,
				Key:  keyVal,
			}
		}
		data.ProviderKeys[provider] = newEntries
	})
}

// DeleteProviderKey removes a single key by ID from a provider's key set.
func (b *SecretsBridge) DeleteProviderKey(provider, keyID string) error {
	return b.store.Update(func(data *SecretData) {
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
func (b *SecretsBridge) MaskedProviderKeys(provider string) []models.APIKeyItem {
	keys := b.GetProviderKeys(provider)
	for i := range keys {
		keys[i].Key = secrets.MaskKey(keys[i].Key)
	}
	return keys
}

// GetSecret (Tool Secret) - for the bridge we map these to the "tools" provider group
func (b *SecretsBridge) GetSecret(category, provider string) string {
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

func (b *SecretsBridge) SetSecret(category, provider, value string) error {
	key := category + ":" + provider
	return b.store.Update(func(data *SecretData) {
		if data.ProviderKeys == nil {
			data.ProviderKeys = make(map[string][]SecretEntry)
		}
		// Try to update existing
		for p, entries := range data.ProviderKeys {
			for i, e := range entries {
				if e.ID == key {
					if secrets.IsMasked(value) {
						value = e.Key
					}
					data.ProviderKeys[p][i].Key = value
					return
				}
			}
		}
		// If not found, add to "tools" provider
		data.ProviderKeys["tools"] = append(data.ProviderKeys["tools"], SecretEntry{
			ID:   key,
			Name: key,
			Key:  value,
		})
	})
}

func (b *SecretsBridge) MaskedSecret(category, provider string) string {
	val := b.GetSecret(category, provider)
	if val == "" {
		return ""
	}
	return secrets.MaskKey(val)
}

func (b *SecretsBridge) GetResolvedProviderKey(provider, name string) (string, error) {
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

func (b *SecretsBridge) ResolveMaskedKey(provider, maskedKey string) (string, error) {
	keys := b.GetProviderKeys(provider)
	if !secrets.IsMasked(maskedKey) {
		return "", fmt.Errorf("provided key is not masked")
	}

	for _, k := range keys {
		if secrets.MaskKey(k.Key) == maskedKey {
			return k.Key, nil
		}
	}

	return "", fmt.Errorf("no key matching mask found for %q", provider)
}

