package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"llm-proxy/internal/platform/secretcrypto"
	"llm-proxy/models"
)

var testMasterKey = []byte("this_is_a_32_byte_key_for_aes_..")

func newTestSecretStore(t *testing.T) (*SecretStore, *Store[models.EncryptedSecretData], string) {
	t.Helper()
	dir := t.TempDir()
	encStore := NewStore[models.EncryptedSecretData](filepath.Join(dir, models.SecretsFilename))
	return NewSecretStore(encStore, testMasterKey), encStore, dir
}

func TestSecretStore_RoundTrip(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)

	keys := []models.APIKeyItem{
		{ID: "k1", Name: "primary", Key: "sk-secret-1", BaseURL: "https://api.example.com"},
		{ID: "k2", Name: "backup", Key: "sk-secret-2"},
	}
	if err := ss.SetProviderKeys("openai", keys); err != nil {
		t.Fatalf("SetProviderKeys: %v", err)
	}

	got := ss.GetProviderKeys("openai")
	if len(got) != 2 {
		t.Fatalf("got %d keys, want 2", len(got))
	}
	if got[0].Key != "sk-secret-1" || got[0].BaseURL != "https://api.example.com" {
		t.Errorf("round-trip mismatch: %+v", got[0])
	}
}

func TestSecretStore_PersistsEncrypted(t *testing.T) {
	ss, _, dir := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-x"}}); err != nil {
		t.Fatal(err)
	}

	// On-disk payload must be ciphertext, not the plaintext key.
	raw, err := os.ReadFile(filepath.Join(dir, models.SecretsFilename))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-x") {
		t.Fatal("plaintext key found on disk in secrets.json")
	}

	// A fresh store over the same file reads the key back (decrypt path).
	ss2 := NewSecretStore(NewStore[models.EncryptedSecretData](filepath.Join(dir, models.SecretsFilename)), testMasterKey)
	if err := ss2.store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := ss2.GetProviderKeys("openai")
	if len(got) != 1 || got[0].Key != "sk-x" {
		t.Errorf("reloaded keys mismatch: %+v", got)
	}
}

func TestSecretStore_MaskedProviderKeys(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-abcdefgh1234"}}); err != nil {
		t.Fatal(err)
	}

	masked := ss.MaskedProviderKeys("openai")
	if len(masked) != 1 || masked[0].Key == "sk-abcdefgh1234" {
		t.Errorf("expected masked key, got %+v", masked)
	}
	if IsMasked(masked[0].Key) != true {
		t.Errorf("expected masked marker, got %q", masked[0].Key)
	}
}

func TestSecretStore_ResolveMaskedKey(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	realKey := "sk-abcdefgh1234"
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: realKey}}); err != nil {
		t.Fatal(err)
	}

	masked := MaskKey(realKey)
	resolved, err := ss.ResolveMaskedKey("openai", masked)
	if err != nil {
		t.Fatalf("ResolveMaskedKey: %v", err)
	}
	if resolved != realKey {
		t.Errorf("resolved = %q, want %q", resolved, realKey)
	}

	if _, err := ss.ResolveMaskedKey("openai", "not-masked"); err == nil {
		t.Error("expected error for unmasked input")
	}
}

func TestSecretStore_MaskedInputKeepsExistingKey(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	realKey := "sk-real-key-value"
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: realKey}}); err != nil {
		t.Fatal(err)
	}

	// Update with a masked value: must resolve to the existing plaintext.
	masked := MaskKey(realKey)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: masked}}); err != nil {
		t.Fatal(err)
	}
	got := ss.GetProviderKeys("openai")
	if got[0].Key != realKey {
		t.Errorf("masked update lost the real key: %q", got[0].Key)
	}
}

func TestSecretStore_DeleteProviderKey(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{
		{ID: "k1", Name: "a", Key: "s1"},
		{ID: "k2", Name: "b", Key: "s2"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := ss.DeleteProviderKey("openai", "k1"); err != nil {
		t.Fatal(err)
	}
	got := ss.GetProviderKeys("openai")
	if len(got) != 1 || got[0].ID != "k2" {
		t.Errorf("after delete got %+v", got)
	}

	if err := ss.DeleteAllProviderKeys("openai"); err != nil {
		t.Fatal(err)
	}
	if got := ss.GetProviderKeys("openai"); len(got) != 0 {
		t.Errorf("after DeleteAll got %+v", got)
	}
}

func TestSecretStore_ToolSecrets(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	if err := ss.SetSecret("search", "brave", "tok-123"); err != nil {
		t.Fatal(err)
	}
	if got := ss.GetSecret("search", "brave"); got != "tok-123" {
		t.Errorf("GetSecret = %q, want tok-123", got)
	}
	if got := ss.MaskedSecret("search", "brave"); got == "tok-123" {
		t.Error("MaskedSecret returned the plaintext")
	}

	// Update an existing tool secret.
	if err := ss.SetSecret("search", "brave", "tok-456"); err != nil {
		t.Fatal(err)
	}
	if got := ss.GetSecret("search", "brave"); got != "tok-456" {
		t.Errorf("GetSecret after update = %q", got)
	}
}

func TestSecretStore_GetResolvedProviderKey(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{
		{ID: "k1", Name: "primary", Key: "key-1"},
		{ID: "k2", Name: "backup", Key: "key-2"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := ss.GetResolvedProviderKey("openai", "backup")
	if err != nil || got != "key-2" {
		t.Errorf("by name: got %q err %v", got, err)
	}
	got, err = ss.GetResolvedProviderKey("openai", "")
	if err != nil || got != "key-1" {
		t.Errorf("default: got %q err %v", got, err)
	}
	_, err = ss.GetResolvedProviderKey("openai", "nope")
	if err == nil {
		t.Error("expected error for unknown key name")
	}
	_, err = ss.GetResolvedProviderKey("nonexistent", "")
	if err == nil {
		t.Error("expected error for unknown provider")
	}

	info, err := ss.GetResolvedProviderKeyInfo("openai", "backup")
	if err != nil || info.Key != "key-2" || info.BaseURL != "" {
		t.Errorf("info: got %+v err %v", info, err)
	}
}

func TestSecretStore_WrongKeyReturnsEmptyNotPanic(t *testing.T) {
	ss, _, _ := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "s1"}}); err != nil {
		t.Fatal(err)
	}

	wrong := NewSecretStore(NewStore[models.EncryptedSecretData](filepath.Join(t.TempDir(), "x.json")), []byte("another_32_byte_key_for_tests!!"))
	// Wrong store has no data yet; decrypt fails and must degrade to empty.
	if got := wrong.GetProviderKeys("openai"); len(got) != 0 {
		t.Errorf("expected empty for wrong-key/empty store, got %+v", got)
	}
}

func TestSecretStore_UpdateFiresOnChange(t *testing.T) {
	ss, encStore, _ := newTestSecretStore(t)

	var calls int32
	encStore.OnChange(func(data models.EncryptedSecretData) { atomic.AddInt32(&calls, 1) })

	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "s1"}}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("OnChange calls = %d, want 1", got)
	}
}

func TestSecretStore_ReloadRoundTripWithVersion(t *testing.T) {
	ss, _, dir := newTestSecretStore(t)
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "s1"}}); err != nil {
		t.Fatal(err)
	}

	// Reload through a fresh store + fresh key from file to verify the full
	// encrypted round-trip (version + ciphertext + nonce).
	ss2 := NewSecretStore(NewStore[models.EncryptedSecretData](filepath.Join(dir, models.SecretsFilename)), testMasterKey)
	if err := ss2.store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := ss2.GetProviderKeys("openai"); len(got) != 1 || got[0].Key != "s1" {
		t.Errorf("reloaded keys mismatch: %+v", got)
	}

	// Sanity: the stored ciphertext really is AES-GCM sealed.
	edata := ss2.store.Get()
	if _, err := secretcrypto.DecryptAES(testMasterKey, edata.Ciphertext, edata.Nonce); err != nil {
		t.Errorf("stored payload does not decrypt: %v", err)
	}
}

// TestSecretStore_SaveToleratesUndecryptableExisting proves that a save
// succeeds even when the on-disk secrets.json was encrypted under a different
// (rotated) master key — the stale ciphertext is discarded instead of
// aborting the write. This is the scenario hit after a key change / factory
// reset, where updateEncrypted must mirror getDecrypted's graceful handling.
func TestSecretStore_SaveToleratesUndecryptableExisting(t *testing.T) {
	ss, encStore, dir := newTestSecretStore(t)

	// Seed an on-disk secrets.json encrypted with a DIFFERENT key.
	otherKey := []byte("a_different_32_byte_key_for_test")
	plaintext, err := json.Marshal(map[string][]models.SecretEntry{"stale": {{ID: "old", Key: "deadbeef"}}})
	if err != nil {
		t.Fatal(err)
	}
	cipher, nonce, err := secretcrypto.EncryptAES(otherKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeExclusiveSecrets(t, dir, cipher, nonce); err != nil {
		t.Fatal(err)
	}
	if err := encStore.Load(); err != nil {
		t.Fatal(err)
	}

	// Saving new keys must NOT fail with a decrypt error.
	if err := ss.SetProviderKeys("openai", []models.APIKeyItem{{ID: "k1", Name: "a", Key: "sk-new"}}); err != nil {
		t.Fatalf("SetProviderKeys with undecryptable existing failed: %v", err)
	}

	got := ss.GetProviderKeys("openai")
	if len(got) != 1 || got[0].Key != "sk-new" {
		t.Errorf("expected new key only, got %+v", got)
	}
}

func writeExclusiveSecrets(t *testing.T, dir string, cipher, nonce string) error {
	t.Helper()
	payload, err := json.MarshalIndent(models.EncryptedSecretData{
		Version:    models.SecretVersionCurrent,
		Ciphertext: cipher,
		Nonce:      nonce,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, models.SecretsFilename), payload, 0600)
}
