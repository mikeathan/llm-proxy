package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func GetMasterKey() ([]byte, error) {
	// 1. Check environment variable
	if keyHex := os.Getenv("LLM_MASTER_KEY"); keyHex != "" {
		key, err := hex.DecodeString(keyHex)
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}

	// 2. Check ~/.config/llm-proxy/master.key
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get home dir: %w", err)
	}

	configDir := filepath.Join(home, ".config", "llm-proxy")
	keyFile := filepath.Join(configDir, "master.key")

	data, err := os.ReadFile(keyFile)
	if err == nil {
		key, err := hex.DecodeString(string(data))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("master key file at %s is corrupted or invalid", keyFile)
		}

		// Integrity Check: verify hash if it exists
		hashFile := keyFile + ".hash"
		if hashData, err := os.ReadFile(hashFile); err == nil {
			expectedHash := hex.EncodeToString(sha256Hash(key))
			if string(hashData) != expectedHash {
				return nil, fmt.Errorf("master key integrity check failed: hash mismatch")
			}
		} else if os.IsNotExist(err) {
			// Backfill hash if missing
			_ = os.WriteFile(hashFile, []byte(hex.EncodeToString(sha256Hash(key))), 0600)
		}

		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read master key: %w", err)
	}

	// 3. Generate new key
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}

	keyHex := hex.EncodeToString(newKey)
	if err := os.WriteFile(keyFile, []byte(keyHex), 0600); err != nil {
		return nil, fmt.Errorf("failed to write master key: %w", err)
	}

	// Write hash for future integrity checks
	_ = os.WriteFile(keyFile+".hash", []byte(hex.EncodeToString(sha256Hash(newKey))), 0600)

	fmt.Printf("[SECURITY] Generated new master key and integrity hash at %s\n", keyFile)
	return newKey, nil
}

func sha256Hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
