package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// Derive Key from passphrase using SHA-256
func deriveKey(passphrase string) []byte {
	hasher := sha256.New()
	hasher.Write([]byte(passphrase))
	return hasher.Sum(nil)
}

func Encrypt(plainText, passphrase string) (string, error) {
	if passphrase == "" {
		return plainText, nil // Fallback to raw if no key is set
	}

	key := deriveKey(passphrase)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return hex.EncodeToString(cipherText), nil
}

func Decrypt(cipherTextHex, passphrase string) (string, error) {
	if passphrase == "" {
		return cipherTextHex, nil // Fallback to raw if no key is set
	}

	key := deriveKey(passphrase)
	cipherText, err := hex.DecodeString(cipherTextHex)
	if err != nil {
		// If decoding fails, check if it's already a raw token (migration safety)
		return cipherTextHex, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherText) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, actualCipherText := cipherText[:nonceSize], cipherText[nonceSize:]
	plainTextBytes, err := gcm.Open(nil, nonce, actualCipherText, nil)
	if err != nil {
		// Fallback for migration (if it was stored as raw)
		return cipherTextHex, nil
	}

	return string(plainTextBytes), nil
}
