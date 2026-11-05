// Package secrets encrypts integration credentials with AES-256-GCM envelope
// encryption. Each secret gets its own data key, wrapped by a versioned master key
// that comes from the environment and never reaches the database.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ErrNotConfigured means no master key is set, so credentials can't be saved or read.
var ErrNotConfigured = errors.New("SECRET_ENCRYPTION_KEY is not set")

const (
	keySize    = 32
	nonceSize  = 12
	wrappedKey = keySize + 16 // data key plus the GCM tag
	format     = 1
)

// Keyring holds the current master key and any older ones still needed for reading.
type Keyring struct {
	current int
	keys    map[int][]byte
}

// NewKeyring builds a keyring. current must be one of the versions in keys.
func NewKeyring(current int, keys map[int][]byte) (*Keyring, error) {
	if _, ok := keys[current]; !ok {
		return nil, fmt.Errorf("no key for version %d", current)
	}
	for v, k := range keys {
		if len(k) != keySize {
			return nil, fmt.Errorf("key version %d must be %d bytes", v, keySize)
		}
	}
	return &Keyring{current, keys}, nil
}

func decodeKey(name, value string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64", name)
	}
	return key, nil
}

// FromEnv reads SECRET_ENCRYPTION_KEY (base64, 32 bytes) and its version. During a
// rotation SECRET_ENCRYPTION_KEY_PREVIOUS holds the key one version older.
func FromEnv() (*Keyring, error) {
	raw := os.Getenv("SECRET_ENCRYPTION_KEY")
	if strings.TrimSpace(raw) == "" {
		return nil, ErrNotConfigured
	}
	version := 1
	if v := strings.TrimSpace(os.Getenv("SECRET_ENCRYPTION_KEY_VERSION")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, errors.New("SECRET_ENCRYPTION_KEY_VERSION must be a positive number")
		}
		version = n
	}
	key, err := decodeKey("SECRET_ENCRYPTION_KEY", raw)
	if err != nil {
		return nil, err
	}
	keys := map[int][]byte{version: key}
	if prev := os.Getenv("SECRET_ENCRYPTION_KEY_PREVIOUS"); strings.TrimSpace(prev) != "" {
		if version < 2 {
			return nil, errors.New("SECRET_ENCRYPTION_KEY_VERSION must be at least 2 when a previous key is set")
		}
		old, err := decodeKey("SECRET_ENCRYPTION_KEY_PREVIOUS", prev)
		if err != nil {
			return nil, err
		}
		keys[version-1] = old
	}
	return NewKeyring(version, keys)
}

// Version is the key version new secrets are encrypted with.
func (k *Keyring) Version() int { return k.current }

func seal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, sealed, aad []byte) ([]byte, error) {
	if len(sealed) < nonceSize {
		return nil, errors.New("ciphertext is too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, sealed[:nonceSize], sealed[nonceSize:], aad)
}

// Encrypt returns the ciphertext and the key version used. aad ties the ciphertext
// to its record, so a stored secret can't be copied onto another connection.
func (k *Keyring) Encrypt(plaintext, aad []byte) ([]byte, int, error) {
	dataKey := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return nil, 0, err
	}
	wrapped, err := seal(k.keys[k.current], dataKey, aad)
	if err != nil {
		return nil, 0, err
	}
	body, err := seal(dataKey, plaintext, aad)
	if err != nil {
		return nil, 0, err
	}
	out := make([]byte, 0, 1+len(wrapped)+len(body))
	out = append(out, format)
	out = append(out, wrapped...)
	return append(out, body...), k.current, nil
}

// Decrypt reverses Encrypt. Failures never include any secret material.
func (k *Keyring) Decrypt(ciphertext []byte, version int, aad []byte) ([]byte, error) {
	master, ok := k.keys[version]
	if !ok {
		return nil, fmt.Errorf("no key for version %d", version)
	}
	const head = 1 + nonceSize + wrappedKey
	if len(ciphertext) < head || ciphertext[0] != format {
		return nil, errors.New("ciphertext has an unknown format")
	}
	dataKey, err := open(master, ciphertext[1:head], aad)
	if err != nil {
		return nil, errors.New("could not decrypt the secret")
	}
	plaintext, err := open(dataKey, ciphertext[head:], aad)
	if err != nil {
		return nil, errors.New("could not decrypt the secret")
	}
	return plaintext, nil
}
