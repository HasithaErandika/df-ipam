package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, keySize) }

func ring(t *testing.T, current int, keys map[int][]byte) *Keyring {
	t.Helper()
	k, err := NewKeyring(current, keys)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	k := ring(t, 1, map[int][]byte{1: key(1)})
	sealed, version, err := k.Encrypt([]byte("api-token"), []byte("conn-1"))
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || bytes.Contains(sealed, []byte("api-token")) {
		t.Fatalf("version %d, ciphertext shows the secret: %v", version, bytes.Contains(sealed, []byte("api-token")))
	}
	got, err := k.Decrypt(sealed, version, []byte("conn-1"))
	if err != nil || string(got) != "api-token" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestEachEncryptionDiffers(t *testing.T) {
	k := ring(t, 1, map[int][]byte{1: key(1)})
	a, _, _ := k.Encrypt([]byte("same"), nil)
	b, _, _ := k.Encrypt([]byte("same"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of one secret match")
	}
}

func TestCiphertextIsBoundToItsRecord(t *testing.T) {
	k := ring(t, 1, map[int][]byte{1: key(1)})
	sealed, v, _ := k.Encrypt([]byte("secret"), []byte("conn-1"))
	if _, err := k.Decrypt(sealed, v, []byte("conn-2")); err == nil {
		t.Fatal("a secret copied to another record decrypted")
	}
}

func TestTamperingIsDetected(t *testing.T) {
	k := ring(t, 1, map[int][]byte{1: key(1)})
	sealed, v, _ := k.Encrypt([]byte("secret"), nil)
	for _, i := range []int{1, 20, len(sealed) - 1} {
		bad := bytes.Clone(sealed)
		bad[i] ^= 1
		if _, err := k.Decrypt(bad, v, nil); err == nil {
			t.Errorf("byte %d changed without detection", i)
		}
	}
	if _, err := k.Decrypt(sealed[:10], v, nil); err == nil {
		t.Error("short ciphertext accepted")
	}
}

func TestRotationReadsOldAndWritesNew(t *testing.T) {
	old := ring(t, 1, map[int][]byte{1: key(1)})
	sealed, v, _ := old.Encrypt([]byte("secret"), nil)

	rotated := ring(t, 2, map[int][]byte{1: key(1), 2: key(2)})
	got, err := rotated.Decrypt(sealed, v, nil)
	if err != nil || string(got) != "secret" {
		t.Fatalf("old secret unreadable after rotation: %v", err)
	}
	if _, nv, _ := rotated.Encrypt(got, nil); nv != 2 {
		t.Fatalf("new secrets use version %d", nv)
	}
	finished := ring(t, 2, map[int][]byte{2: key(2)})
	if _, err := finished.Decrypt(sealed, v, nil); err == nil {
		t.Fatal("a retired key version still decrypts")
	}
}

func TestFromEnv(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString
	t.Setenv("SECRET_ENCRYPTION_KEY", "")
	t.Setenv("SECRET_ENCRYPTION_KEY_VERSION", "")
	t.Setenv("SECRET_ENCRYPTION_KEY_PREVIOUS", "")
	if _, err := FromEnv(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", b64([]byte("short")))
	if _, err := FromEnv(); err == nil {
		t.Fatal("short key accepted")
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", "not base64!")
	if _, err := FromEnv(); err == nil {
		t.Fatal("invalid base64 accepted")
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", b64(key(2)))
	t.Setenv("SECRET_ENCRYPTION_KEY_PREVIOUS", b64(key(1)))
	if _, err := FromEnv(); err == nil {
		t.Fatal("previous key accepted at version 1")
	}
	t.Setenv("SECRET_ENCRYPTION_KEY_VERSION", "2")
	k, err := FromEnv()
	if err != nil || k.Version() != 2 || len(k.keys) != 2 {
		t.Fatalf("keyring %+v, %v", k, err)
	}
}
