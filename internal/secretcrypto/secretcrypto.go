package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const formatVersion byte = 1

var associatedData = []byte("deploycrate-ce/secret/v1")

func Encrypt(plaintext []byte, hexKey string) ([]byte, error) {
	return encrypt(plaintext, hexKey, associatedData)
}

func EncryptForPurpose(plaintext []byte, hexKey, purpose string) ([]byte, error) {
	if purpose == "" {
		return nil, errors.New("secret encryption purpose is required")
	}
	return encrypt(plaintext, hexKey, []byte("deploycrate-ce/secret/v1/"+purpose))
}

func encrypt(plaintext []byte, hexKey string, aad []byte) ([]byte, error) {
	key, err := decodeKey(hexKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secret AEAD: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate secret nonce: %w", err)
	}
	sealed := make([]byte, 1, 1+len(nonce)+len(plaintext)+aead.Overhead())
	sealed[0] = formatVersion
	sealed = append(sealed, nonce...)
	sealed = aead.Seal(sealed, nonce, plaintext, aad)
	return sealed, nil
}

func Decrypt(ciphertext []byte, hexKey string) ([]byte, error) {
	return decrypt(ciphertext, hexKey, associatedData)
}

func DecryptForPurpose(ciphertext []byte, hexKey, purpose string) ([]byte, error) {
	if purpose == "" {
		return nil, errors.New("secret decryption purpose is required")
	}
	return decrypt(ciphertext, hexKey, []byte("deploycrate-ce/secret/v1/"+purpose))
}

func decrypt(ciphertext []byte, hexKey string, aad []byte) ([]byte, error) {
	if len(ciphertext) == 0 || ciphertext[0] != formatVersion {
		return nil, errors.New("unsupported encrypted secret format")
	}
	key, err := decodeKey(hexKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secret AEAD: %w", err)
	}
	payload := ciphertext[1:]
	if len(payload) < aead.NonceSize() {
		return nil, errors.New("encrypted secret is truncated")
	}
	nonce, payload := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, payload, aad)
	if err != nil {
		return nil, errors.New("decrypt secret")
	}
	return plaintext, nil
}

func decodeKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, errors.New("secret encryption key must be hex encoded")
	}
	if len(key) != 32 {
		return nil, errors.New("secret encryption key must be 32 bytes")
	}
	return key, nil
}
