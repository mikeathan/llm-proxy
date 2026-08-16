// Package secretcrypto provides the AES-256-GCM envelope used to encrypt
// secrets.json. It lives in its own package (below both paths and storage) so
// the paths package can seed an encrypted empty secrets file at first run
// without importing storage.
package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptAES seals plaintext with AES-256-GCM and returns base64-encoded
// ciphertext and nonce. A fresh random nonce is generated per call.
func EncryptAES(key []byte, plaintext []byte) (string, string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce), nil
}

// DecryptAES opens an AES-256-GCM sealed blob produced by EncryptAES.
func DecryptAES(key []byte, ciphertextB64 string, nonceB64 string) ([]byte, error) {
	if ciphertextB64 == "" || nonceB64 == "" {
		return nil, errors.New("empty ciphertext or nonce")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
