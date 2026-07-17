package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

const (
	fileSuffix     = ".cookiex"
	envelopeFormat = 1
)

var validProfileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Profile is an encrypted, reproducible snapshot of cookies from one browser
// profile for one target host.
type Profile struct {
	Name               string               `json:"name"`
	Host               string               `json:"host"`
	Browser            string               `json:"browser"`
	BrowserProfile     string               `json:"browser_profile"`
	BrowserProfilePath string               `json:"browser_profile_path"`
	CreatedAt          time.Time            `json:"created_at"`
	SyncedAt           time.Time            `json:"synced_at"`
	Cookies            []cookiemodel.Cookie `json:"cookies"`
	Headers            map[string]string    `json:"headers,omitempty"`
}

type KeyProvider interface {
	MasterKey() ([]byte, error)
}

type Store struct {
	dir  string
	keys KeyProvider
}

type envelope struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func New(dir string, keys KeyProvider) *Store {
	return &Store{dir: dir, keys: keys}
}

func (s *Store) Save(profile Profile) error {
	if err := validateName(profile.Name); err != nil {
		return err
	}
	if err := s.ensureDir(); err != nil {
		return err
	}

	plaintext, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode profile %q: %w", profile.Name, err)
	}
	aead, err := s.aead()
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate profile nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, []byte(profile.Name))
	encoded, err := json.Marshal(envelope{
		Version:    envelopeFormat,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return fmt.Errorf("encode encrypted profile %q: %w", profile.Name, err)
	}
	return atomicWrite(filepath.Join(s.dir, profile.Name+fileSuffix), encoded)
}

func (s *Store) Load(name string) (Profile, error) {
	if err := validateName(name); err != nil {
		return Profile{}, err
	}
	encoded, err := os.ReadFile(filepath.Join(s.dir, name+fileSuffix))
	if err != nil {
		return Profile{}, fmt.Errorf("read profile %q: %w", name, err)
	}
	var container envelope
	if err := json.Unmarshal(encoded, &container); err != nil {
		return Profile{}, fmt.Errorf("decode profile envelope %q: %w", name, err)
	}
	if container.Version != envelopeFormat {
		return Profile{}, fmt.Errorf("profile %q uses unsupported format %d", name, container.Version)
	}
	nonce, err := base64.StdEncoding.DecodeString(container.Nonce)
	if err != nil {
		return Profile{}, fmt.Errorf("decode profile nonce %q: %w", name, err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(container.Ciphertext)
	if err != nil {
		return Profile{}, fmt.Errorf("decode profile ciphertext %q: %w", name, err)
	}
	aead, err := s.aead()
	if err != nil {
		return Profile{}, err
	}
	if len(nonce) != aead.NonceSize() {
		return Profile{}, fmt.Errorf("profile %q has invalid nonce length", name)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(name))
	if err != nil {
		return Profile{}, fmt.Errorf("authenticate profile %q: %w", name, err)
	}
	var profile Profile
	if err := json.Unmarshal(plaintext, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode profile %q: %w", name, err)
	}
	if profile.Name != name {
		return Profile{}, fmt.Errorf("profile name mismatch: file %q contains %q", name, profile.Name)
	}
	return profile, nil
}

func (s *Store) Exists(name string) (bool, error) {
	if err := validateName(name); err != nil {
		return false, err
	}
	_, err := os.Stat(filepath.Join(s.dir, name+fileSuffix))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("check profile %q: %w", name, err)
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileSuffix) {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), fileSuffix))
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) aead() (cipher.AEAD, error) {
	if s.keys == nil {
		return nil, errors.New("vault key provider is not configured")
	}
	key, err := s.keys.MasterKey()
	if err != nil {
		return nil, fmt.Errorf("get vault master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("vault master key has length %d, want 32", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize vault cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create vault directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("protect vault directory: %w", err)
	}
	return nil
}

func validateName(name string) error {
	if !validProfileName.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid profile name %q", name)
	}
	return nil
}

func atomicWrite(path string, data []byte) (returnErr error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".cookiex-*")
	if err != nil {
		return fmt.Errorf("create temporary profile: %w", err)
	}
	tempPath := file.Name()
	defer func() {
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect temporary profile: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary profile: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary profile: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary profile: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace profile: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect profile: %w", err)
	}
	return nil
}
