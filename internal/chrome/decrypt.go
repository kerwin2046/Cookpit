package chrome

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	chromeSalt = []byte("saltysalt")
	chromeIV   = bytes.Repeat([]byte{' '}, aes.BlockSize)
)

// SecretProvider retrieves Chrome's OSCrypt password from Secret Service.
type SecretProvider interface {
	Secret(application string) (string, error)
}

// LinuxDecrypter decrypts the legacy v10/v11 OSCrypt format used by Chrome
// and Chromium on Linux.
type LinuxDecrypter struct {
	application string
	secrets     SecretProvider
}

func NewLinuxDecrypter(application string, secrets SecretProvider) *LinuxDecrypter {
	return &LinuxDecrypter{application: application, secrets: secrets}
}

func (d *LinuxDecrypter) Decrypt(encrypted []byte, host string, dbVersion int) (string, error) {
	if len(encrypted) < 3 {
		return string(encrypted), nil
	}

	prefix := string(encrypted[:3])
	if prefix != "v10" && prefix != "v11" {
		return string(encrypted), nil
	}

	password := "peanuts"
	if prefix == "v11" {
		if d.secrets == nil {
			return "", errors.New("Chrome v11 cookie requires Linux Secret Service")
		}
		secret, err := d.secrets.Secret(d.application)
		if err != nil {
			return "", fmt.Errorf("read %s Safe Storage secret: %w", d.application, err)
		}
		if secret == "" {
			return "", errors.New("Chrome Safe Storage secret is empty")
		}
		password = secret
	}

	ciphertext := encrypted[3:]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid %s ciphertext length %d", prefix, len(ciphertext))
	}

	key := pbkdf2SHA1([]byte(password), chromeSalt, 1, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialize AES: %w", err)
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, chromeIV).CryptBlocks(plaintext, ciphertext)

	plaintext, err = unpadPKCS7(plaintext, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return decodePayload(plaintext, host, dbVersion)
}

func decodePayload(payload []byte, host string, dbVersion int) (string, error) {
	if dbVersion < 24 {
		return string(payload), nil
	}
	if len(payload) < sha256.Size {
		return "", fmt.Errorf("version %d cookie payload is shorter than domain hash", dbVersion)
	}
	want := sha256.Sum256([]byte(host))
	if subtle.ConstantTimeCompare(payload[:sha256.Size], want[:]) != 1 {
		return "", fmt.Errorf("cookie domain hash does not match %q", host)
	}
	return string(payload[sha256.Size:]), nil
}

func unpadPKCS7(value []byte, blockSize int) ([]byte, error) {
	if len(value) == 0 || len(value)%blockSize != 0 {
		return nil, errors.New("invalid PKCS#7 payload length")
	}
	padding := int(value[len(value)-1])
	if padding == 0 || padding > blockSize || padding > len(value) {
		return nil, errors.New("invalid PKCS#7 padding")
	}
	for _, b := range value[len(value)-padding:] {
		if int(b) != padding {
			return nil, errors.New("invalid PKCS#7 padding")
		}
	}
	return value[:len(value)-padding], nil
}

func pbkdf2SHA1(password, salt []byte, iterations, length int) []byte {
	output := make([]byte, 0, length)
	for blockIndex := uint32(1); len(output) < length; blockIndex++ {
		mac := hmac.New(sha1.New, password)
		_, _ = mac.Write(salt)
		var index [4]byte
		binary.BigEndian.PutUint32(index[:], blockIndex)
		_, _ = mac.Write(index[:])
		u := mac.Sum(nil)
		block := append([]byte(nil), u...)

		for iteration := 1; iteration < iterations; iteration++ {
			mac = hmac.New(sha1.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for i := range block {
				block[i] ^= u[i]
			}
		}
		output = append(output, block...)
	}
	return output[:length]
}
