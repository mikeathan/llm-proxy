package secretcrypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptAES(t *testing.T) {
	key := []byte("this_is_a_32_byte_key_for_aes_..") // 32 bytes
	plaintext := []byte("hello world JSON payload")

	cipherB64, nonceB64, err := EncryptAES(key, plaintext)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if cipherB64 == "" || nonceB64 == "" {
		t.Fatal("Expected non-empty ciphertext and nonce")
	}

	decrypted, err := DecryptAES(key, cipherB64, nonceB64)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Expected %q, got %q", string(plaintext), string(decrypted))
	}
}

func TestDecryptAES_InvalidKey(t *testing.T) {
	key1 := []byte("this_is_a_32_byte_key_for_aes_..")
	key2 := []byte("this_is_another_32_byte_key.....")
	plaintext := []byte("sensitive info")

	cipherB64, nonceB64, _ := EncryptAES(key1, plaintext)

	_, err := DecryptAES(key2, cipherB64, nonceB64)
	if err == nil {
		t.Fatal("Expected decryption to fail with wrong key")
	}
}
