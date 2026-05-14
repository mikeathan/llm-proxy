package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

type EncryptedSecretData struct {
	Version    int    `json:"version"`
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
}

func main() {
	keyHex := "6de432a74e90f607ff2753368707334fbfc972c47d5979922ed393c0acde2546"
	key, _ := hex.DecodeString(keyHex)

	data, err := os.ReadFile("data/secrets.json")
	if err != nil {
		fmt.Printf("Read file error: %v\n", err)
		return
	}
	var edata EncryptedSecretData
	if err := json.Unmarshal(data, &edata); err != nil {
		fmt.Printf("Unmarshal error: %v\n", err)
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(edata.Ciphertext)
	if err != nil {
		fmt.Printf("Decode ciphertext error: %v\n", err)
		return
	}
	nonce, err := base64.StdEncoding.DecodeString(edata.Nonce)
	if err != nil {
		fmt.Printf("Decode nonce error: %v\n", err)
		return
	}

	fmt.Printf("Nonce length: %d\n", len(nonce))

	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Printf("NewCipher error: %v\n", err)
		return
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Printf("NewGCM error: %v\n", err)
		return
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		fmt.Printf("Open error: %v\n", err)
		return
	}

	fmt.Println(string(plaintext))
}
