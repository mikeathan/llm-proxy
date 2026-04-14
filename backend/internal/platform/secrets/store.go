// Package secrets provides secure, file-based storage for sensitive data
// that must not be co-located with the public configuration (config.json).
//
// The store writes to a secrets.json file with 0600 permissions (owner read/write only),
// preventing accidental exposure via VCS commits or shared config files.
//
// Design principles:
//   - The interface is consumer-defined; callers program to Store, not fileStore.
//   - The UI receives masked keys (sk-...1234). On save, the backend re-hydrates
//     any masked value with its true stored key, preventing accidental overwrites.
//   - Atomic writes are used to prevent partial-write corruption.
//   - A single RWMutex protects in-memory state; disk I/O happens under the write lock.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"llm-proxy/models"
)

// ErrNotFound is returned when a secret entry does not exist.
var ErrNotFound = errors.New("secret not found")

type Store interface {
	// Provider API keys (LLM Providers)
	GetProviderKeys(provider string) []models.APIKeyItem
	SetProviderKeys(provider string, keys []models.APIKeyItem) error
	DeleteProviderKey(provider, keyID string) error
	MaskedProviderKeys(provider string) []models.APIKeyItem

	// Tool Secrets (Search, Communication, etc.)
	// category: "search", "communication", etc.
	// provider: "tavily", "telegram", etc.
	GetSecret(category, provider string) string
	SetSecret(category, provider, value string) error
	MaskedSecret(category, provider string) string

	// GetResolvedProviderKey looks up a key by name or ID.
	// If name is empty, it returns the first available key for the provider.
	GetResolvedProviderKey(provider, name string) (string, error)

	// ResolveMaskedKey finds a real key that matches the provided mask string.
	ResolveMaskedKey(provider, maskedKey string) (string, error)
}

type secretsFile struct {
	Version      int                             `json:"version"`
	ProviderKeys map[string][]models.APIKeyItem    `json:"provider_keys,omitempty"`
	ToolSecrets  map[string]map[string]string      `json:"tool_secrets,omitempty"` // map[category][provider]value
}

type fileStore struct {
	path string
	mu   sync.RWMutex
	data secretsFile
}

// NewFileStore creates a new file-backed secrets store rooted at dir.
// The directory is created with 0700 permissions (owner-only access).
// The secrets file itself is created with 0600 permissions.
func NewFileStore(dir string) (Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("secrets: failed to create directory: %w", err)
	}

	path := filepath.Join(dir, "secrets.json")
	store := &fileStore{
		path: path,
		data: secretsFile{
			Version:      1,
			ProviderKeys: make(map[string][]models.APIKeyItem),
		},
	}

	if err := store.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("secrets: failed to load: %w", err)
		}
		// File doesn't exist yet — write the empty skeleton so permissions are set correctly.
		if err := store.save(); err != nil {
			return nil, fmt.Errorf("secrets: failed to initialize store: %w", err)
		}
	}

	return store, nil
}

// load reads and deserializes the secrets file into memory.
// Called once during construction, under no lock (safe at init time).
func (s *fileStore) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	var data secretsFile
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty file — initialize with defaults.
			return nil
		}
		return fmt.Errorf("secrets: failed to decode: %w", err)
	}

	if data.ProviderKeys == nil {
		data.ProviderKeys = make(map[string][]models.APIKeyItem)
	}
	if data.ToolSecrets == nil {
		data.ToolSecrets = make(map[string]map[string]string)
	}

	s.data = data
	return nil
}

// save atomically writes the current in-memory state to disk with 0600 permissions,
// using a write-then-rename strategy to avoid partial-write corruption.
// Must be called with s.mu held (write lock).
func (s *fileStore) save() error {
	dir := filepath.Dir(s.path)
	// Write to a temp file in the same directory so rename is atomic on the same fs.
	tmp, err := os.CreateTemp(dir, ".secrets-*.json.tmp")
	if err != nil {
		return fmt.Errorf("secrets: failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Set permissions before writing any data.
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("secrets: failed to set file permissions: %w", err)
	}

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("secrets: failed to encode: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("secrets: failed to flush temp file: %w", err)
	}

	// Atomic rename onto the final path.
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("secrets: failed to commit secrets file: %w", err)
	}

	return nil
}

// GetProviderKeys returns a copy of all API keys for the given provider.
func (s *fileStore) GetProviderKeys(provider string) []models.APIKeyItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys, ok := s.data.ProviderKeys[provider]
	if !ok {
		return []models.APIKeyItem{}
	}
	out := make([]models.APIKeyItem, len(keys))
	copy(out, keys)
	return out
}

// MaskedProviderKeys returns the provider keys with their secret values redacted.
// This is safe to send to the frontend.
func (s *fileStore) MaskedProviderKeys(provider string) []models.APIKeyItem {
	keys := s.GetProviderKeys(provider)
	for i := range keys {
		keys[i].Key = MaskKey(keys[i].Key)
	}
	return keys
}

// SetProviderKeys replaces the full key set for a provider.
// If any incoming key carries a masked value (e.g., "sk-...1234"), it is
// automatically hydrated from the existing stored key for that ID, ensuring
// a round-trip through the masked UI never silently discards the real key.
func (s *fileStore) SetProviderKeys(provider string, keys []models.APIKeyItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build a lookup map for existing keys by ID.
	existingByID := make(map[string]string, len(s.data.ProviderKeys[provider]))
	for _, k := range s.data.ProviderKeys[provider] {
		existingByID[k.ID] = k.Key
	}

	final := make([]models.APIKeyItem, len(keys))
	for i, incoming := range keys {
		final[i] = incoming
		if IsMasked(incoming.Key) {
			real, ok := existingByID[incoming.ID]
			if !ok {
				return fmt.Errorf("secrets: masked key %q has no stored counterpart", incoming.ID)
			}
			final[i].Key = real
		}
	}

	s.data.ProviderKeys[provider] = final
	return s.save()
}

// DeleteProviderKey removes a single key by ID from a provider's key set.
func (s *fileStore) DeleteProviderKey(provider, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, ok := s.data.ProviderKeys[provider]
	if !ok {
		return ErrNotFound
	}

	filtered := keys[:0]
	found := false
	for _, k := range keys {
		if k.ID == keyID {
			found = true
			continue
		}
		filtered = append(filtered, k)
	}

	if !found {
		return ErrNotFound
	}

	s.data.ProviderKeys[provider] = filtered
	return s.save()
}

func (s *fileStore) GetSecret(category, provider string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.ToolSecrets == nil {
		return ""
	}
	if cat, ok := s.data.ToolSecrets[category]; ok {
		return cat[provider]
	}
	return ""
}

func (s *fileStore) SetSecret(category, provider, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Hydrate if masked.
	if IsMasked(value) {
		if s.data.ToolSecrets != nil {
			if cat, ok := s.data.ToolSecrets[category]; ok {
				if real, ok := cat[provider]; ok {
					value = real
				}
			}
		}
	}

	if s.data.ToolSecrets == nil {
		s.data.ToolSecrets = make(map[string]map[string]string)
	}
	if _, ok := s.data.ToolSecrets[category]; !ok {
		s.data.ToolSecrets[category] = make(map[string]string)
	}
	s.data.ToolSecrets[category][provider] = value
	return s.save()
}

func (s *fileStore) MaskedSecret(category, provider string) string {
	val := s.GetSecret(category, provider)
	if val == "" {
		return ""
	}
	return MaskKey(val)
}

func (s *fileStore) GetResolvedProviderKey(provider, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match first
	keys, ok := s.data.ProviderKeys[provider]
	if !ok {
		// Try case-insensitive fallback
		for k, v := range s.data.ProviderKeys {
			if strings.EqualFold(k, provider) {
				keys = v
				ok = true
				break
			}
		}
	}

	if !ok || len(keys) == 0 {
		return "", fmt.Errorf("%w: no keys found for provider %q", ErrNotFound, provider)
	}

	// If a specific name/ID was requested, try to find it.
	if name != "" {
		for _, k := range keys {
			if k.Name == name || k.ID == name {
				return k.Key, nil
			}
		}
		// If we asked for a specific name and it's NOT found, that's an error.
		return "", fmt.Errorf("%w: key %q not found for provider %q", ErrNotFound, name, provider)
	}

	// Default to first available key.
	return keys[0].Key, nil
}

// ResolveMaskedKey finds a real key that matches the provided mask string.
func (s *fileStore) ResolveMaskedKey(provider, maskedKey string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Direct match check in keys list
	keys := s.data.ProviderKeys[provider]
	if !IsMasked(maskedKey) {
		return "", fmt.Errorf("provided key is not masked")
	}

	for _, k := range keys {
		if MaskKey(k.Key) == maskedKey {
			return k.Key, nil
		}
	}

	return "", fmt.Errorf("%w: no key matching mask %q found for %q", ErrNotFound, maskedKey, provider)
}

// MaskKey redacts a secret key, preserving only a short prefix and suffix
// for identification. Example: "sk-mysecretkey12345" → "sk-...2345"
func MaskKey(key string) string {
	const (
		prefixLen = 4
		suffixLen = 4
	)
	if len(key) <= prefixLen+suffixLen {
		return "***"
	}
	return key[:prefixLen] + "..." + key[len(key)-suffixLen:]
}

// IsMasked returns true if a key string appears to be a redacted placeholder
// rather than a real credential.
func IsMasked(key string) bool {
	return strings.Contains(key, "...") || 
	       strings.Contains(key, "***") || 
	       strings.Contains(key, "•••") || 
	       strings.Contains(key, "●●●")
}
