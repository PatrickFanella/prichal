package enrollment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
)

// Cipher protects transient enrollment codes independently from host credentials.
type Cipher struct{ aead cipher.AEAD }

func LoadCipher(path string) (*Cipher, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 32 {
		return nil, errors.New("private enrollment encryption key required")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("enrollment encryption key unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead}, nil
}
func (c *Cipher) seal(code, scope string) ([]byte, error) {
	if c == nil || len(code) == 0 || len(code) > 256 {
		return nil, errors.New("enrollment encryption unavailable")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, []byte(code), []byte(scope)), nil
}
func (c *Cipher) open(encoded []byte, scope string) (string, error) {
	if c == nil || len(encoded) < c.aead.NonceSize()+c.aead.Overhead() || len(encoded) > 512 {
		return "", errors.New("protected enrollment code unavailable")
	}
	size := c.aead.NonceSize()
	code, err := c.aead.Open(nil, encoded[:size], encoded[size:], []byte(scope))
	if err != nil {
		return "", errors.New("protected enrollment code could not be authenticated")
	}
	return string(code), nil
}
