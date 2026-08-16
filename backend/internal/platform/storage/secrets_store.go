package storage

import (
	"encoding/json"
	"fmt"
	"sync"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/secretcrypto"
	"llm-proxy/models"
)

// SecretStore implements models.SecretsStore using a technical Store[models.EncryptedSecretData].
// It handles high-level logic like masking, hydration, credential resolution, and AES encryption.
type SecretStore struct {
	store     *Store[models.EncryptedSecretData]
	masterKey []byte

	// decrypted is a cache of the decrypted view (P1). getDecrypted no longer
	// AES-decrypts and unmarshals on every call — the request hot path only
	// pays the cost after a Load or Update invalidates the cache.
	mu        sync.RWMutex
	decrypted *models.SecretData
}

func NewSecretStore(store *Store[models.EncryptedSecretData], masterKey []byte) *SecretStore {
	s := &SecretStore{store: store, masterKey: masterKey}
	// Invalidate the cache whenever the underlying encrypted store changes
	// (external edits via the watcher, our own writes).
	store.OnChange(func(models.EncryptedSecretData) {
		s.invalidate()
	})
	return s
}

// invalidate drops the cached decrypted view. Safe for concurrent callers.
func (b *SecretStore) invalidate() {
	b.mu.Lock()
	b.decrypted = nil
	b.mu.Unlock()
}

func (b *SecretStore) getDecrypted() models.SecretData {
	b.mu.RLock()
	if b.decrypted != nil {
		cached := *b.decrypted
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	// Double-check after acquiring the write lock.
	if b.decrypted != nil {
		return *b.decrypted
	}

	edata := b.store.Get()
	if edata.Ciphertext == "" {
		data := models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
		b.decrypted = &data
		return data
	}

	if edata.Version > models.SecretVersionCurrent {
		logging.Warn("secrets.json version is newer than supported; decrypting best-effort",
			"version", edata.Version, "supported", models.SecretVersionCurrent)
	}

	plaintext, err := secretcrypto.DecryptAES(b.masterKey, edata.Ciphertext, edata.Nonce)
	if err != nil {
		logging.Warn("failed to decrypt secrets json", "error", err)
		data := models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
		b.decrypted = &data
		return data
	}

	var data models.SecretData
	if err := json.Unmarshal(plaintext, &data.ProviderKeys); err != nil {
		logging.Warn("failed to unmarshal decrypted secrets", "error", err)
		data = models.SecretData{Version: edata.Version, ProviderKeys: make(map[string][]models.SecretEntry)}
		b.decrypted = &data
		return data
	}
	data.Version = edata.Version
	if data.ProviderKeys == nil {
		data.ProviderKeys = make(map[string][]models.SecretEntry)
	}
	b.decrypted = &data
	return data
}

func (b *SecretStore) updateEncrypted(fn func(data *models.SecretData)) error {
	b.invalidate()

	return b.store.Update(func(edata *models.EncryptedSecretData) error {
		var data models.SecretData
		// Decrypt existing credentials. A ciphertext that cannot be decrypted
		// (e.g. the master key was rotated, or the file is corrupt) is treated as
		// empty rather than aborting the write — we are about to overwrite it
		// anyway. This mirrors getDecrypted's graceful handling and keeps the
		// secrets UI usable after a key change / factory reset.
		if edata.Ciphertext != "" {
			plaintext, err := secretcrypto.DecryptAES(b.masterKey, edata.Ciphertext, edata.Nonce)
			if err != nil {
				logging.Warn("existing secrets ciphertext undecryptable; starting from empty store",
					"error", err)
			} else if err := json.Unmarshal(plaintext, &data.ProviderKeys); err != nil {
				logging.Warn("existing secrets ciphertext unmarshalable; starting from empty store",
					"error", err)
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

		cipher, nonce, err := secretcrypto.EncryptAES(b.masterKey, plaintext)
		if err != nil {
			return fmt.Errorf("failed to encrypt updated secrets: %w", err)
		}

		edata.Version = models.SecretVersionCurrent
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
					logging.Warn("could not resolve masked key",
						"id", k.ID, "provider", provider)
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

// resolveKey finds the matching API key entry for a provider. With an empty
// name it returns the first key (the default credential); with a name it
// matches Name or ID. It backs both GetResolvedProviderKey and
// GetResolvedProviderKeyInfo so the linear scan lives in one place.
func (b *SecretStore) resolveKey(provider, name string) (models.APIKeyItem, error) {
	keys := b.GetProviderKeys(provider)
	if len(keys) == 0 {
		return models.APIKeyItem{}, fmt.Errorf("provider %q not found", provider)
	}

	if name != "" {
		for _, k := range keys {
			if k.Name == name || k.ID == name {
				return k, nil
			}
		}
		return models.APIKeyItem{}, fmt.Errorf("key %q not found for provider %q", name, provider)
	}

	return keys[0], nil
}

func (b *SecretStore) GetResolvedProviderKey(provider, name string) (string, error) {
	k, err := b.resolveKey(provider, name)
	if err != nil {
		return "", err
	}
	return k.Key, nil
}

func (b *SecretStore) GetResolvedProviderKeyInfo(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
	k, err := b.resolveKey(provider, name)
	if err != nil {
		return nil, err
	}
	return &models.ResolvedProviderKeyInfo{Key: k.Key, BaseURL: k.BaseURL}, nil
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
