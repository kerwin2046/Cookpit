package vault

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "cookiex"
	keyringUser    = "vault-master-key"
)

var errSecretNotFound = errors.New("secret not found")

type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) {
	value, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errSecretNotFound
	}
	return value, err
}

func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

// KeyringKeyProvider keeps Cookiex's random vault key in Linux Secret Service.
type KeyringKeyProvider struct {
	backend keyringBackend
	mu      sync.Mutex
}

func NewKeyringKeyProvider() *KeyringKeyProvider {
	return newKeyringKeyProvider(systemKeyring{})
}

func newKeyringKeyProvider(backend keyringBackend) *KeyringKeyProvider {
	return &KeyringKeyProvider{backend: backend}
}

func (p *KeyringKeyProvider) MasterKey() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	encoded, err := p.backend.Get(keyringService, keyringUser)
	if err == nil {
		key, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode vault key from Secret Service: %w", decodeErr)
		}
		return key, nil
	}
	if !errors.Is(err, errSecretNotFound) {
		return nil, fmt.Errorf("read vault key from Secret Service: %w", err)
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate vault key: %w", err)
	}
	encoded = base64.StdEncoding.EncodeToString(key)
	if err := p.backend.Set(keyringService, keyringUser, encoded); err != nil {
		return nil, fmt.Errorf("store vault key in Secret Service: %w", err)
	}
	return key, nil
}
