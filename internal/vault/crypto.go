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
	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iter = 210000
	saltLen    = 16
	dekLen     = 32
)

// Cipher derives a per-account AES-256-GCM key from the server master key, so
// a leaked row of one account never decrypts with another account's key.
// When a password-wrapped DEK is installed, EncryptItem/DecryptItem prefer it.
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
	return c.encryptWith(accountID, nil, plaintext)
}

func (c *Cipher) Decrypt(accountID uuid.UUID, ciphertext, nonce []byte) (string, error) {
	return c.decryptWith(accountID, nil, ciphertext, nonce)
}

// EncryptWithDEK encrypts using a password-unwrapped data encryption key.
func (c *Cipher) EncryptWithDEK(accountID uuid.UUID, dek []byte, plaintext string) (ciphertext, nonce []byte, err error) {
	return c.encryptWith(accountID, dek, plaintext)
}

func (c *Cipher) DecryptWithDEK(accountID uuid.UUID, dek []byte, ciphertext, nonce []byte) (string, error) {
	return c.decryptWith(accountID, dek, ciphertext, nonce)
}

func (c *Cipher) encryptWith(accountID uuid.UUID, dek []byte, plaintext string) ([]byte, []byte, error) {
	key := dek
	var err error
	if len(key) == 0 {
		key, err = c.accountKey(accountID)
		if err != nil {
			return nil, nil, err
		}
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, []byte(plaintext), accountID[:]), nonce, nil
}

func (c *Cipher) decryptWith(accountID uuid.UUID, dek []byte, ciphertext, nonce []byte) (string, error) {
	try := func(key []byte) (string, error) {
		gcm, err := newGCM(key)
		if err != nil {
			return "", err
		}
		pt, err := gcm.Open(nil, nonce, ciphertext, accountID[:])
		if err != nil {
			return "", err
		}
		return string(pt), nil
	}
	if len(dek) > 0 {
		if s, err := try(dek); err == nil {
			return s, nil
		}
	}
	key, err := c.accountKey(accountID)
	if err != nil {
		return "", err
	}
	s, err := try(key)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return s, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func deriveKEK(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, pbkdf2Iter, 32, sha256.New)
}

// NewWrappedDEK creates a random DEK wrapped by a password-derived KEK.
func NewWrappedDEK(password string) (salt, wrapped, nonce, dek []byte, err error) {
	salt = make([]byte, saltLen)
	if _, err = rand.Read(salt); err != nil {
		return
	}
	dek = make([]byte, dekLen)
	if _, err = rand.Read(dek); err != nil {
		return
	}
	kek := deriveKEK(password, salt)
	gcm, err := newGCM(kek)
	if err != nil {
		return
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return
	}
	wrapped = gcm.Seal(nil, nonce, dek, nil)
	return
}

// UnwrapDEK recovers the DEK using the account password.
func UnwrapDEK(password string, salt, wrapped, nonce []byte) ([]byte, error) {
	if len(salt) == 0 || len(wrapped) == 0 || len(nonce) == 0 {
		return nil, errors.New("vault password wrap not configured")
	}
	kek := deriveKEK(password, salt)
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	dek, err := gcm.Open(nil, nonce, wrapped, nil)
	if err != nil {
		return nil, errors.New("invalid vault password")
	}
	return dek, nil
}
