package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	_ "embed"
)

type Encryption struct {
	password string
	iter     int
}

func (e *Encryption) passwordEncrypt(plaintext []byte, password string) ([]byte, error) {
	var p string
	p = e.password
	if password != "" {
		p = password
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("error generating salt: %v", err)
	}
	key, err := pbkdf2.Key(sha256.New, p, salt, e.iter, 32)
	if err != nil {
		return nil, fmt.Errorf("error generating pbkdf2 key: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("NewGCM: %s", err)
	}
	iv := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate iv: %s", err)
	}
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)
	hexSalt := make([]byte, hex.EncodedLen(len(salt)))
	hex.Encode(hexSalt, salt)
	hexIv := make([]byte, hex.EncodedLen(len(iv)))
	hex.Encode(hexIv, iv)
	hexCiphertext := make([]byte, hex.EncodedLen(len(ciphertext)))
	hex.Encode(hexCiphertext, ciphertext)
	return append(append(append(append(hexSalt, []byte("-")...), hexIv...), []byte("-")...), hexCiphertext...), nil // lol
}

func (e *Encryption) passwordDecrypt(cipherText []byte, password string) ([]byte, error) {
	var p string
	p = e.password
	if password != "" {
		p = password
	}
	if !bytes.ContainsAny(cipherText, "-") {
		return nil, errors.New("invalid data")
	}
	data := bytes.Split(cipherText, []byte("-"))
	salt := make([]byte, hex.DecodedLen(len(data[0])))
	if _, err := hex.Decode(salt, data[0]); err != nil {
		return nil, errors.New("invalid data: salt" + err.Error())
	}
	iv := make([]byte, hex.DecodedLen(len(data[1])))
	if _, err := hex.Decode(iv, data[1]); err != nil {
		return nil, errors.New("invalid data: iv" + err.Error())
	}
	ciphertext := make([]byte, hex.DecodedLen(len(data[2])))
	if _, err := hex.Decode(ciphertext, data[2]); err != nil {
		return nil, errors.New("invalid data: ciphertext" + err.Error())
	}
	key, err := pbkdf2.Key(sha256.New, p, salt, e.iter, 32)
	if err != nil {
		return nil, errors.New("error generating pbkdf2 key:" + err.Error())
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("decrypt failed:" + err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("decrypt failed:" + err.Error())
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decrypt failed:" + err.Error())
	}
	return plaintext, nil
}
