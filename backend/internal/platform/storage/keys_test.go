package storage

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/internal/platform/paths"
)

func TestGetMasterKey_AfterSeeding(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	key, err := GetMasterKey(dir)
	if err != nil {
		t.Fatalf("GetMasterKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}

	// File holds a 64-char hex string, not raw bytes.
	data, err := os.ReadFile(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 64 {
		t.Errorf("master.key is %d bytes, want 64-char hex", len(data))
	}
	if hex.EncodeToString(key) != strings.TrimSpace(string(data)) {
		t.Error("GetMasterKey returned a key different from the file")
	}
}

func TestGetMasterKey_ReloadSameKey(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatal(err)
	}

	k1, err := GetMasterKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := GetMasterKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(k1) != string(k2) {
		t.Error("reload returned a different key")
	}
}

func TestGetMasterKey_SeedDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatal(err)
	}
	k1, _ := GetMasterKey(dir)

	// Second seed must not overwrite the existing key (O_EXCL semantics).
	if err := p.SeedDefaults(); err != nil {
		t.Fatal(err)
	}
	k2, _ := GetMasterKey(dir)
	if string(k1) != string(k2) {
		t.Error("SeedDefaults overwrote the existing master key")
	}
}

func TestGetMasterKey_EnvPath(t *testing.T) {
	dir := t.TempDir()
	envKey := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	t.Setenv("LLM_MASTER_KEY", envKey)
	t.Cleanup(func() { _ = os.Unsetenv("LLM_MASTER_KEY") })

	key, err := GetMasterKey(dir)
	if err != nil {
		t.Fatalf("GetMasterKey: %v", err)
	}
	if hex.EncodeToString(key) != envKey {
		t.Errorf("key = %x, want env key", key)
	}
}

func TestGetMasterKey_InvalidEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LLM_MASTER_KEY", "not-a-valid-key")
	t.Cleanup(func() { _ = os.Unsetenv("LLM_MASTER_KEY") })

	if _, err := GetMasterKey(dir); err == nil {
		t.Fatal("expected error for invalid LLM_MASTER_KEY")
	}
}

func TestGetMasterKey_TamperDetection(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{ConfigDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatal(err)
	}

	// Corrupt the hash so the integrity check fails.
	if err := os.WriteFile(filepath.Join(dir, "master.key.hash"), []byte("deadbeef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GetMasterKey(dir); err == nil {
		t.Fatal("expected integrity error on tampered hash")
	} else if !strings.Contains(err.Error(), "integrity") {
		t.Errorf("expected integrity error, got: %v", err)
	}
}

func TestGetMasterKey_CorruptKeyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "master.key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GetMasterKey(dir); err == nil {
		t.Fatal("expected error for corrupt key file")
	}
}

func TestGetMasterKey_MissingIsLoud(t *testing.T) {
	// No seed → key file missing → loud error (C4), never a silent nil key.
	dir := t.TempDir()
	if _, err := GetMasterKey(dir); err == nil {
		t.Fatal("expected loud error when master key is missing")
	}
}
