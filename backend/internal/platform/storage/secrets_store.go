package storage

import (
	"encoding/json"
	"fmt"
	"llm-proxy/models"
)

// SecretStore implements models.SecretsStore using a technical Store[models.EncryptedSecretData].
// It handles high-level logic like masking, hydration, credential resolution, and AES encryption.
type SecretStore struct {
	store     *Store[models.EncryptedSecretData]
	masterKey []byte
}

func NewSecretStore(store *Store[models.EncryptedSecretData], masterKey []byte) *SecretStore {
	return &SecretStore{store: store, masterKey: masterKey}
}

func (b *SecretStore) getDecrypted() models.SecretData {
	edata := b.store.Get()
	if edata.Ciphertext == "" {
		return models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
	}

	plaintext, err := DecryptAES(b.masterKey, edata.Ciphertext, edata.Nonce)
	if err != nil {
		fmt.Printf("Warning: Failed to decrypt secrets json: %v\n", err)
		return models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
	}

	var data models.SecretData
	if err := json.Unmarshal(plaintext, &data.ProviderKeys); err != nil {
		fmt.Printf("Warning: Failed to unmarshal decrypted secrets: %v\n", err)
		return models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
	}
	data.Version = edata.Version
	if data.ProviderKeys == nil {
		data.ProviderKeys = make(map[string][]models.SecretEntry)
	}
	return data
}

func (b *SecretStore) updateEncrypted(fn func(data *models.SecretData)) error {
	return b.store.Update(func(edata *models.EncryptedSecretData) error {
		var data models.SecretData
		// Decrypt
		if edata.Ciphertext != "" {
			plaintext, err := DecryptAES(b.masterKey, edata.Ciphertext, edata.Nonce)
			if err != nil {
				return fmt.Errorf("failed to decrypt secrets: %w", err)
			}
			if err := json.Unmarshal(plaintext, &data.ProviderKeys); err != nil {
				return fmt.Errorf("failed to unmarshal secrets: %w", err)
			}
		}

		if data.ProviderKeys == nil {
			data.ProviderKeys = make(map[string][]models.SecretEntry)
		}

		// Apply callback
		fn(&data)

		// Encrypt back
		plaintext, err := json.Marshal(data.ProviderKeys)
		if err != nil {
			return fmt.Errorf("failed to marshal updated secrets: %w", err)
		}

		cipher, nonce, err := EncryptAES(b.masterKey, plaintext)
		if err != nil {
			return fmt.Errorf("failed to encrypt updated secrets: %w", err)
		}

		edata.Ciphertext = cipher
		edata.Nonce = nonce
		return nil
	})
}

func (b *SecretStore) GetProviderKeys(provider string) []models.APIKeyItem {
	data := b.getDecrypted()
	entries, ok := data.ProviderKeys[provider]
	if !ok {
		return []models.APIKeyItem{}
	}
	res := make([]models.APIKeyItem, len(entries))
	for i, e := range entries {
		res[i] = models.APIKeyItem{
			ID:      e.ID,
			Name:    e.Name,
			Key:     e.Key,
			BaseURL: e.BaseURL,
		}
	}
	return res
}

func (b *SecretStore) SetProviderKeys(provider string, keys []models.APIKeyItem) error {
	return b.updateEncrypted(func(data *models.SecretData) {
		// Build lookup from existing keys for this provider
		type existingInfo struct {
			key     string
			baseURL string
		}
		existingByID := make(map[string]existingInfo)
		if providerKeys, ok := data.ProviderKeys[provider]; ok {
			for _, e := range providerKeys {
				existingByID[e.ID] = existingInfo{key: e.Key, baseURL: e.BaseURL}
			}
		}

		newEntries := make([]models.SecretEntry, len(keys))
		for i, k := range keys {
			keyVal := k.Key
			baseURL := k.BaseURL
			if IsMasked(keyVal) {
				if existing, ok := existingByID[k.ID]; ok {
					keyVal = existing.key
				} else {
					fmt.Printf("Warning: could not resolve masked key for ID %s in provider %s\n", k.ID, provider)
				}
			}
			if baseURL == "" {
				if existing, ok := existingByID[k.ID]; ok {
					baseURL = existing.baseURL
				}
			}
			newEntries[i] = models.SecretEntry{
				ID:      k.ID,
				Name:    k.Name,
				Key:     keyVal,
				BaseURL: baseURL,
			}
		}
		data.ProviderKeys[provider] = newEntries
	})
}

func (b *SecretStore) DeleteProviderKey(provider, keyID string) error {
	return b.updateEncrypted(func(data *models.SecretData) {
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

func (b *SecretStore) DeleteAllProviderKeys(provider string) error {
	return b.SetProviderKeys(provider, []models.APIKeyItem{})
}

func (b *SecretStore) MaskedProviderKeys(provider string) []models.APIKeyItem {
	keys := b.GetProviderKeys(provider)
	for i := range keys {
		keys[i].Key = MaskKey(keys[i].Key)
	}
	return keys
}

// GetSecret (Tool Secret) - for the bridge we map these to the "tools" provider group
func (b *SecretStore) GetSecret(category, provider string) string {
	data := b.getDecrypted()
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
	return b.updateEncrypted(func(data *models.SecretData) {
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

func (b *SecretStore) GetResolvedProviderKeyInfo(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
	keys := b.GetProviderKeys(provider)
	if len(keys) == 0 {
		return nil, fmt.Errorf("provider %q not found", provider)
	}

	if name != "" {
		for _, k := range keys {
			if k.Name == name || k.ID == name {
				return &models.ResolvedProviderKeyInfo{
					Key:     k.Key,
					BaseURL: k.BaseURL,
				}, nil
			}
		}
		return nil, fmt.Errorf("key %q not found for provider %q", name, provider)
	}

	return &models.ResolvedProviderKeyInfo{
		Key:     keys[0].Key,
		BaseURL: keys[0].BaseURL,
	}, nil
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
