package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"golang.org/x/crypto/scrypt"
)

func deriveKey(password string, salt []byte) (key, salt2 []byte, err error) {
	if salt == nil {
		salt = make([]byte, 32)
		if _, err = rand.Read(salt); err != nil {
			return
		}
	}
	key, err = scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	salt2 = salt
	return
}

func encrypt(data, key []byte) (enc []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return
	}
	enc = gcm.Seal(nonce, nonce, data, nil)
	return
}

func decrypt(enc, key []byte) (data []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return
	}
	nonce := enc[:gcm.NonceSize()]
	enc = enc[gcm.NonceSize():]
	return gcm.Open(nil, nonce, enc, nil)
}
