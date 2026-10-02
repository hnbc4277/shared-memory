package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Cipher derives a per-account AES-256-GCM key from the server master key, so
// a leaked row of one account never decrypts with another account's key.
type Cipher struct {
	master []byte
}

func NewCipher(masterKey string) (*Cipher, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("VAULT_MASTER_KEY must be at least 32 characters")
	}
	return &Cipher{master: []byte(masterKey)}, nil
}

func (c *Cipher) accountKey(accountID uuid.UUID) ([]byte, error) {
	return hkdf.Key(sha256.New, c.master, accountID[:], "shared-memory/vault/v1", 32)
}

func (c *Cipher) Encrypt(accountID uuid.UUID, plaintext string) (ciphertext, nonce []byte, err error) {
	key, err := c.accountKey(accountID)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, []byte(plaintext), accountID[:]), nonce, nil
}

func (c *Cipher) Decrypt(accountID uuid.UUID, ciphertext, nonce []byte) (string, error) {
	key, err := c.accountKey(accountID)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ciphertext, accountID[:])
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
