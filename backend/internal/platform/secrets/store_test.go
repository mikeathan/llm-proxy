package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"llm-proxy/models"
)

func TestNewFileStore_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore returned error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}

	// Verify the file exists.
	path := filepath.Join(dir, "secrets.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("secrets.json was not created: %v", err)
	}

	// Verify 0600 on Unix-like systems (skip on Windows).
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0600 {
			t.Fatalf("expected permission 0600, got %v", info.Mode().Perm())
		}
	}
}

func TestFileStore_SetAndGet(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())

	keys := []models.APIKeyItem{
		{ID: "id-1", Name: "Production", Key: "sk-realkey1234567890"},
	}
	if err := store.SetProviderKeys("openai", keys); err != nil {
		t.Fatalf("SetProviderKeys: %v", err)
	}

	got := store.GetProviderKeys("openai")
	if len(got) != 1 || got[0].Key != "sk-realkey1234567890" {
		t.Fatalf("unexpected keys: %+v", got)
	}
}

func TestFileStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)

	_ = store.SetProviderKeys("gemini", []models.APIKeyItem{
		{ID: "id-2", Name: "Dev", Key: "AIzasecretdevkey"},
	})

	// Re-open from disk.
	store2, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	got := store2.GetProviderKeys("gemini")
	if len(got) != 1 || got[0].Key != "AIzasecretdevkey" {
		t.Fatalf("persisted key not found: %+v", got)
	}
}

func TestFileStore_MaskedKeys(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	_ = store.SetProviderKeys("openrouter", []models.APIKeyItem{
		{ID: "id-3", Name: "Main", Key: "sk-or-v1-reallylongsecretkey"},
	})

	masked := store.MaskedProviderKeys("openrouter")
	if len(masked) != 1 {
		t.Fatalf("expected 1 masked key, got %d", len(masked))
	}
	if !isMasked(masked[0].Key) {
		t.Fatalf("expected masked key, got: %q", masked[0].Key)
	}
	if masked[0].Key == "sk-or-v1-reallylongsecretkey" {
		t.Fatal("real key should not be returned in masked response")
	}
}

func TestFileStore_SetMaskedDoesNotOverwriteRealKey(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	original := models.APIKeyItem{ID: "id-4", Name: "Prod", Key: "sk-realsecretkey99999"}
	_ = store.SetProviderKeys("openai", []models.APIKeyItem{original})

	// Simulate what the frontend sends back after loading masked keys.
	masked := store.MaskedProviderKeys("openai")
	if err := store.SetProviderKeys("openai", masked); err != nil {
		t.Fatalf("SetProviderKeys with masked input: %v", err)
	}

	// The real key must be preserved.
	got := store.GetProviderKeys("openai")
	if len(got) != 1 || got[0].Key != "sk-realsecretkey99999" {
		t.Fatalf("real key was overwritten by masked value: %+v", got)
	}
}

func TestFileStore_DeleteProviderKey(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	_ = store.SetProviderKeys("openai", []models.APIKeyItem{
		{ID: "id-5", Name: "Key5", Key: "sk-todelete"},
		{ID: "id-6", Name: "Key6", Key: "sk-tokeep"},
	})

	if err := store.DeleteProviderKey("openai", "id-5"); err != nil {
		t.Fatalf("DeleteProviderKey: %v", err)
	}

	got := store.GetProviderKeys("openai")
	if len(got) != 1 || got[0].ID != "id-6" {
		t.Fatalf("unexpected keys after delete: %+v", got)
	}
}

func TestFileStore_DeleteNotFound(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	err := store.DeleteProviderKey("openai", "nonexistent")
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
}

func TestFileStore_GetEmptyProvider(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	got := store.GetProviderKeys("nvidia")
	if got == nil || len(got) != 0 {
		t.Fatalf("expected empty non-nil slice, got: %+v", got)
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input    string
		wantMask bool
	}{
		{"sk-short", false}, // ≤ 8 chars → "***"
		{"sk-reallylongkey123456", true},
	}

	for _, tt := range tests {
		masked := MaskKey(tt.input)
		got := isMasked(masked)
		if got != tt.wantMask {
			t.Errorf("MaskKey(%q) = %q, isMasked=%v, want %v", tt.input, masked, got, tt.wantMask)
		}
	}
}

func TestFileStore_ToolSecrets(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())

	// Test set/get
	if err := store.SetSecret("search", "tavily", "tvly-real-abc-123"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	got := store.GetSecret("search", "tavily")
	if got != "tvly-real-abc-123" {
		t.Fatalf("expected tvly-real-abc-123, got %q", got)
	}

	// Test masking
	masked := store.MaskedSecret("search", "tavily")
	if !isMasked(masked) {
		t.Fatalf("expected masked secret, got %q", masked)
	}

	// Test hydration (don't overwrite with masked value)
	if err := store.SetSecret("search", "tavily", masked); err != nil {
		t.Fatalf("SetSecret masked: %v", err)
	}
	got2 := store.GetSecret("search", "tavily")
	if got2 != "tvly-real-abc-123" {
		t.Fatalf("real secret was overwritten by masked value: %q", got2)
	}
}

func TestFileStore_GetResolvedProviderKey(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())

	keys := []models.APIKeyItem{
		{ID: "id-1", Name: "Key-1", Key: "secret-1"},
		{ID: "id-2", Name: "Key-2", Key: "secret-2"},
	}
	_ = store.SetProviderKeys("openai", keys)

	// Test lookup by name
	key, _ := store.GetResolvedProviderKey("openai", "Key-2")
	if key != "secret-2" {
		t.Errorf("expected secret-2, got %q", key)
	}

	// Test lookup by ID
	key, _ = store.GetResolvedProviderKey("openai", "id-1")
	if key != "secret-1" {
		t.Errorf("expected secret-1, got %q", key)
	}

	// Test fallback to first (empty name)
	key, _ = store.GetResolvedProviderKey("openai", "")
	if key != "secret-1" {
		t.Errorf("expected secret-1, got %q", key)
	}

	// Test fallback to first (non-existent name)
	key, _ = store.GetResolvedProviderKey("openai", "invalid")
	if key != "secret-1" {
		t.Errorf("expected secret-1, got %q", key)
	}
}
