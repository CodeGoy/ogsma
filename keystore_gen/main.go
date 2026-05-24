package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	ecies "github.com/ecies/go/v2"
)

type Keystore struct {
	Username   []byte          `json:"username"`
	ID         []byte          `json:"id"`
	PublicKey  []byte          `json:"publicKey"`
	PrivateKey []byte          `json:"privateKey"`
	Contacts   []*StoreContact `json:"contacts"`
}

type StoreContact struct {
	PublicKey []byte `json:"publicKey"`
	ID        []byte `json:"id"`
	Username  []byte `json:"username"`
}

type KeyShare struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	PublicKey string `json:"publicKey"`
}

type Keys struct {
	Username   string `json:"username"`
	ID         string `json:"id"`
	PublicKey  *ecies.PublicKey
	PrivateKey *ecies.PrivateKey
	Contacts   []*Contact // Contacts array of contacts
}

type Contact struct {
	PublicKey *ecies.PublicKey
	ID        string
	Username  string
}

type Encryption struct {
	targetPublicKey *ecies.PublicKey
	keyStoreFile    string
	password        string
	bits            int
	iter            int
	keys            *Keys
}

func (e *Encryption) generateECCKeyPair() (*ecies.PrivateKey, *ecies.PublicKey, error) {
	k, err := ecies.GenerateKey()
	if err != nil {
		panic(err)
	}
	return k, k.PublicKey, nil
}

func (e *Encryption) passwordEncrypt(plaintext []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("error generating salt: %v", err)
	}
	key, err := pbkdf2.Key(sha256.New, e.password, salt, e.iter, 32)
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

func (e *Encryption) passwordDecrypt(cipherText []byte) ([]byte, error) {
	if !bytes.ContainsAny(cipherText, "-") {
		return nil, fmt.Errorf("invalid data")
	}
	data := bytes.Split(cipherText, []byte("-"))
	salt := make([]byte, hex.DecodedLen(len(data[0])))
	if _, err := hex.Decode(salt, data[0]); err != nil {
		return nil, fmt.Errorf("invalid data: salt")
	}
	iv := make([]byte, hex.DecodedLen(len(data[1])))
	if _, err := hex.Decode(iv, data[1]); err != nil {
		return nil, fmt.Errorf("invalid data: iv")
	}
	ciphertext := make([]byte, hex.DecodedLen(len(data[2])))
	if _, err := hex.Decode(ciphertext, data[2]); err != nil {
		return nil, fmt.Errorf("invalid data: ciphertext")
	}
	key, err := pbkdf2.Key(sha256.New, e.password, salt, e.iter, 32)
	if err != nil {
		return nil, fmt.Errorf("error generating pbkdf2 key: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: %v", err)
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: %v", err)
	}
	return plaintext, nil
}

func (e *Encryption) keyGen(username string) {
	privateKey, publicKey, err := e.generateECCKeyPair()
	if err != nil {
		log.Printf("Error generating ecc keys: %v\n", err)
		return
	}
	e.saveKeystore(&Keystore{
		Username:   []byte(username),
		PublicKey:  publicKey.Bytes(false),
		PrivateKey: privateKey.Bytes(),
		Contacts:   []*StoreContact{},
		ID:         []byte(generateRandomString(64)),
	})

}

func (e *Encryption) loadKeys() {
	encryptedKeystoreFileBytes, err := os.ReadFile(e.keyStoreFile)
	if err != nil {
		log.Printf("Error opening keystore: %v\n", err)
	}
	keystoreFileBytes, err := e.passwordDecrypt(encryptedKeystoreFileBytes)
	if err != nil {
		log.Printf("Error decrypting keystore: %v\n", err)
		return
	}
	ks := &Keystore{}
	if err := json.Unmarshal(keystoreFileBytes, ks); err != nil {
		log.Printf("Error unmarshaling keystore: %v\n", err)
	}
	publicKeyFromBytes, err := ecies.NewPublicKeyFromBytes(ks.PublicKey)
	if err != nil {
		log.Printf("Error decrypting public key: %v\n", err)
	}
	e.keys = &Keys{
		Username:   string(ks.Username),
		ID:         string(ks.ID),
		PublicKey:  publicKeyFromBytes,
		PrivateKey: ecies.NewPrivateKeyFromBytes(ks.PrivateKey),
		Contacts:   []*Contact{},
	}
	for _, i := range ks.Contacts {
		contactPublicKeyFromBytes, err := ecies.NewPublicKeyFromBytes(i.PublicKey)
		if err != nil {
			log.Printf("Error decrypting contact: %v\n", err)
		}
		e.keys.Contacts = append(e.keys.Contacts, &Contact{
			PublicKey: contactPublicKeyFromBytes,
			ID:        string(i.ID),
			Username:  string(i.Username),
		})
	}
}

func (e *Encryption) publicEncrypt(plaintext []byte) ([]byte, error) {
	ciphertext, err := ecies.Encrypt(e.targetPublicKey, plaintext)
	if err != nil {
		return nil, errors.New("error encrypting with public key" + err.Error())
	}
	return ciphertext, nil
}

func (e *Encryption) privateDecrypt(ciphertext []byte) ([]byte, error) {
	plaintext, err := ecies.Decrypt(e.keys.PrivateKey, ciphertext)
	if err != nil {
		return nil, errors.New("error decrypting with private key" + err.Error())
	}
	return plaintext, nil
}

func (e *Encryption) addContact(id, un string, publicKey *ecies.PublicKey) {
	ks := &Keystore{
		Username:   []byte(e.keys.Username),
		ID:         []byte(e.keys.ID),
		PublicKey:  e.keys.PublicKey.Bytes(false),
		PrivateKey: e.keys.PrivateKey.Bytes(),
		Contacts:   []*StoreContact{},
	}
	for _, ct := range e.keys.Contacts {
		ks.Contacts = append(ks.Contacts, &StoreContact{PublicKey: ct.PublicKey.Bytes(false), ID: []byte(ct.ID), Username: []byte(ct.Username)})
	}
	ks.Contacts = append(ks.Contacts, &StoreContact{PublicKey: publicKey.Bytes(false), ID: []byte(id), Username: []byte(un)})
	e.saveKeystore(ks)
}

func (e *Encryption) saveKeystore(ks *Keystore) {
	marshaledKeystore, err := json.MarshalIndent(ks, "", " ")
	if err != nil {
		log.Printf("Error marshaling keystore: %v\n", err)
		return
	}
	// TODO : encrypt entire keystore
	encryptedKeystore, err := e.passwordEncrypt(marshaledKeystore)
	if err != nil {
		log.Printf("Error encrypting keystore: %v\n", err)
		return
	}
	if err := os.WriteFile(e.keyStoreFile, encryptedKeystore, 0600); err != nil {
		log.Printf("Error writing keystore: %v\n", err)
	}
}

func (e *Encryption) shareKey(filename string) {
	shs := KeyShare{
		ID:        e.keys.ID,
		Username:  e.keys.Username,
		PublicKey: base64.StdEncoding.EncodeToString(e.keys.PublicKey.Bytes(false)),
	}
	jsonBytes, err := json.MarshalIndent(shs, "", " ")
	if err != nil {
		log.Printf("Error marshaling keyshare: %v\n", err)
	}
	// fmt.Printf("%s\n", jsonBytes)
	// TODO : output filename
	if err := os.WriteFile(fmt.Sprintf("%s", filename), jsonBytes, 0600); err != nil {
		log.Printf("Error writing keyshare: %v\n", err)
	}
}

func (e *Encryption) addContactFromFile(filename string) {
	file, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("Error reading file: %v\n", err)
		return
	}
	nca := KeyShare{}
	if err := json.Unmarshal(file, &nca); err != nil {
		log.Printf("Error unmarshaling file: %v\n", err)
		return
	}
	pkb, err := base64.StdEncoding.DecodeString(nca.PublicKey)
	if err != nil {
		log.Printf("Error decoding public key: %v\n", err)
	}
	contactPublicKey, err := ecies.NewPublicKeyFromBytes(pkb)
	if err != nil {
		log.Printf("Error decoding public key: %v\n", err)
	}
	e.addContact(nca.ID, nca.Username, contactPublicKey)
}

func (e *Encryption) printKeys() {
	fmt.Printf("keys: %+v\n", e.keys)
	for _, k := range e.keys.Contacts {
		fmt.Printf("Contact: %+v\n", k)
	}
}

func main() {
	var password, contactKeyFile, newUsername, keyStoreFilename, keyshareFilename string
	var printKeys, test bool
	flag.BoolVar(&test, "test", false, "test flag")
	flag.BoolVar(&printKeys, "print", false, "print keys")
	flag.StringVar(&password, "password", "", "password to encrypt the keystore")
	flag.StringVar(&newUsername, "new", "", "New user name")
	flag.StringVar(&contactKeyFile, "add", "", "path to contact key file")
	flag.StringVar(&keyStoreFilename, "keystore", "", "path to keystore file")
	flag.StringVar(&keyshareFilename, "keyshare", "", "path to keyshare file")
	flag.Parse()
	e := &Encryption{
		password:     password,
		keyStoreFile: keyStoreFilename,
		bits:         4096,
		iter:         100000,
	}
	if len(newUsername) > 0 {
		e.keyGen(newUsername)
	}
	e.loadKeys()
	if printKeys {
		e.printKeys()
		return
	}
	if len(contactKeyFile) > 0 {
		e.addContactFromFile(contactKeyFile)
		return
	}
	e.shareKey(keyshareFilename)
	if test {
		e.targetPublicKey = e.keys.PublicKey
		ciphertext, err := e.publicEncrypt([]byte("hello world, this is ECC public/private key encryption!"))
		if err != nil {
			panic(err)
		}
		log.Printf("encrypted: %v\n", ciphertext)
		plaintext, err := e.privateDecrypt(ciphertext)
		if err != nil {
			panic(err)
		}
		log.Printf("decrypted: %s\n", string(plaintext))
	}
}
