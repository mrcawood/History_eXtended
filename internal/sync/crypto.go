package sync

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	KeySize   = 32
	NonceSize = 24 // XChaCha20-Poly1305
)

// EncryptPayload encrypts plaintext with a random K_obj, wraps K_obj with K_master.
// aad is the AEAD associated data (e.g. header bytes); pass nil to skip.
// Returns: nonce (24 bytes), ciphertext (includes auth tag), wrappedKey (nonce|wrapped).
func EncryptPayload(K_master, plaintext, aad []byte) (nonce, ciphertext, wrappedKey []byte, err error) {
	K_obj := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, K_obj); err != nil {
		return nil, nil, nil, err
	}
	return encryptWithKey(K_master, K_obj, plaintext, aad)
}

// encryptWithKey performs one-pass encrypt (generates nonce internally).
func encryptWithKey(K_master, K_obj, plaintext, aad []byte) (nonce, ciphertext, wrappedKey []byte, err error) {
	if len(K_master) != KeySize || len(K_obj) != KeySize {
		return nil, nil, nil, fmt.Errorf("keys must be %d bytes", KeySize)
	}
	nonce = make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, nil, err
	}
	aead, err := chacha20poly1305.NewX(K_obj)
	if err != nil {
		return nil, nil, nil, err
	}
	ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	wrappedKey, err = WrapKey(K_master, K_obj)
	return nonce, ciphertext, wrappedKey, err
}

// WrapKey wraps K_obj with K_master. Returns nonce|wrapped.
func WrapKey(K_master, K_obj []byte) ([]byte, error) {
	if len(K_master) != KeySize || len(K_obj) != KeySize {
		return nil, fmt.Errorf("keys must be %d bytes", KeySize)
	}
	wrapAead, err := chacha20poly1305.NewX(K_master)
	if err != nil {
		return nil, err
	}
	wrapNonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return nil, err
	}
	wrapped := wrapAead.Seal(nil, wrapNonce, K_obj, nil)
	return append(wrapNonce, wrapped...), nil
}

// SealWithKey encrypts plaintext with existing K_obj, nonce, and aad. For binding AAD to header.
func SealWithKey(K_obj, nonce, plaintext, aad []byte) ([]byte, error) {
	if len(K_obj) != KeySize || len(nonce) != NonceSize {
		return nil, fmt.Errorf("invalid key or nonce size")
	}
	aead, err := chacha20poly1305.NewX(K_obj)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

// DecryptPayload decrypts using wrapped key and K_master.
func DecryptPayload(K_master, nonce, ciphertext, wrappedKey, headerBytes []byte) ([]byte, error) {
	if len(K_master) != KeySize {
		return nil, fmt.Errorf("K_master must be %d bytes", KeySize)
	}
	// Wrapped = nonce (24) + Seal(32) = 24 + 32 + 16 (tag)
	if len(wrappedKey) < NonceSize+KeySize+16 {
		return nil, fmt.Errorf("wrapped key too short")
	}
	wrapNonce := wrappedKey[:NonceSize]
	wrapped := wrappedKey[NonceSize:]

	wrapAead, err := chacha20poly1305.NewX(K_master)
	if err != nil {
		return nil, err
	}
	K_obj, err := wrapAead.Open(nil, wrapNonce, wrapped, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap key: %w", err)
	}
	if len(K_obj) != KeySize {
		return nil, fmt.Errorf("unwrapped key wrong size")
	}

	aead, err := chacha20poly1305.NewX(K_obj)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, headerBytes)
}

// NonceHex returns hex of nonce for CryptoEnv.
func NonceHex(nonce []byte) string {
	return hex.EncodeToString(nonce)
}

// DecodeNonce decodes hex nonce.
func DecodeNonce(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}
