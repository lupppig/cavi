package youtube

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// seal encrypts a refresh token with AES-256-GCM (nonce first), bound to
// the organisation so a sealed token copied to another row won't open.
func seal(key []byte, orgID, token string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(token), []byte(orgID)), nil
}

// open decrypts what seal made.
func open(key []byte, orgID string, sealed []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("youtube: sealed token too short")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(orgID))
	if err != nil {
		return "", errors.New("youtube: stored token can't be decrypted (wrong CAVI_TOKEN_KEY?)")
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("youtube: token key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
