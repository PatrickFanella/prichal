package enrollment

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, key, 0600); err != nil {
		t.Fatal(err)
	}
	value, err := LoadCipher(path)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestCipherBindsTenantAndRejectsTamperingWithoutReturningCode(t *testing.T) {
	value := testCipher(t)
	code := "local-transient-invitation"
	first, err := value.seal(code, "operation/deployment/organization")
	if err != nil {
		t.Fatal(err)
	}
	second, err := value.seal(code, "operation/deployment/organization")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) || bytes.Contains(first, []byte(code)) {
		t.Fatal("nonce reused or plaintext stored")
	}
	decoded, err := value.open(first, "operation/deployment/organization")
	if err != nil || decoded != code {
		t.Fatal("protected code did not round trip")
	}
	for _, scope := range []string{"other-operation/deployment/organization", "operation/other-deployment/organization", "operation/deployment/other-organization"} {
		decoded, err = value.open(first, scope)
		if err == nil || decoded != "" {
			t.Fatal("code escaped its authorized scope")
		}
	}
	first[len(first)-1] ^= 1
	decoded, err = value.open(first, "operation/deployment/organization")
	if err == nil || decoded != "" {
		t.Fatal("unauthenticated code returned")
	}
	decoded, err = testCipher(t).open(second, "operation/deployment/organization")
	if err == nil || decoded != "" {
		t.Fatal("different key authenticated code")
	}
}
func TestCipherRefusesUnsafeAndMissingKeys(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "key")
	if _, err := LoadCipher(path); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(path, make([]byte, 32), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCipher(path); err == nil {
		t.Fatal("public key file accepted")
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCipher(link); err == nil {
		t.Fatal("key symlink followed")
	}
}
