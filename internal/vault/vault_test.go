package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

type staticKeyProvider struct {
	key []byte
	err error
}

func (p staticKeyProvider) MasterKey() ([]byte, error) {
	return append([]byte(nil), p.key...), p.err
}

type memoryKeyring struct {
	value string
}

func (m *memoryKeyring) Get(string, string) (string, error) {
	if m.value == "" {
		return "", errSecretNotFound
	}
	return m.value, nil
}

func (m *memoryKeyring) Set(_, _, value string) error {
	m.value = value
	return nil
}

func TestSaveLoadAndListEncryptedProfile(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	profile := sampleProfile()

	if err := store.Save(profile); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, "work.cookiex")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("super-secret")) {
		t.Fatal("vault file contains plaintext cookie value")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("profile mode = %o, want 600", got)
	}

	got, err := store.Load("work")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != profile.Name || len(got.Cookies) != 1 || got.Cookies[0].Value != "super-secret" {
		t.Fatalf("loaded profile = %#v", got)
	}

	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 1 || names[0] != "work" {
		t.Fatalf("List = %v, want [work]", names)
	}
}

func TestSaveUsesANewNonce(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	if err := store.Save(sampleProfile()); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "work.cookiex"))
	if err := store.Save(sampleProfile()); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, "work.cookiex"))
	if bytes.Equal(first, second) {
		t.Fatal("two saves produced identical ciphertext")
	}
}

func TestLoadRejectsTamperedCiphertext(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	if err := store.Save(sampleProfile()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "work.cookiex")
	raw, _ := os.ReadFile(path)
	raw[len(raw)-1] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("work"); err == nil {
		t.Fatal("Load accepted tampered ciphertext")
	}
}

func TestStoreRejectsUnsafeProfileName(t *testing.T) {
	store := New(t.TempDir(), staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	profile := sampleProfile()
	profile.Name = "../escape"
	if err := store.Save(profile); err == nil {
		t.Fatal("Save accepted path traversal profile name")
	}
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("Load accepted path traversal profile name")
	}
}

func TestSaveLoadPreservesHeaders(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	profile := sampleProfile()
	profile.Headers = map[string]string{
		"x-vis-domain": "www.compamed-tradefair.com",
		"Accept":       "application/json",
	}
	if err := store.Save(profile); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load("work")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Headers["x-vis-domain"] != "www.compamed-tradefair.com" || got.Headers["Accept"] != "application/json" {
		t.Fatalf("headers = %#v", got.Headers)
	}
}

func TestStoreExists(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, staticKeyProvider{key: bytes.Repeat([]byte{7}, 32)})
	exists, err := store.Exists("work")
	if err != nil || exists {
		t.Fatalf("Exists before save = %v, %v", exists, err)
	}
	if err := store.Save(sampleProfile()); err != nil {
		t.Fatal(err)
	}
	exists, err = store.Exists("work")
	if err != nil || !exists {
		t.Fatalf("Exists after save = %v, %v", exists, err)
	}
}

func TestStorePropagatesKeyProviderError(t *testing.T) {
	wantErr := errors.New("keyring unavailable")
	store := New(t.TempDir(), staticKeyProvider{err: wantErr})
	if err := store.Save(sampleProfile()); !errors.Is(err, wantErr) {
		t.Fatalf("Save error = %v, want wrapped %v", err, wantErr)
	}
}

func TestKeyringProviderCreatesAndReusesMasterKey(t *testing.T) {
	backend := &memoryKeyring{}
	provider := newKeyringKeyProvider(backend)

	first, err := provider.MasterKey()
	if err != nil {
		t.Fatalf("first MasterKey: %v", err)
	}
	second, err := provider.MasterKey()
	if err != nil {
		t.Fatalf("second MasterKey: %v", err)
	}
	if len(first) != 32 {
		t.Fatalf("key length = %d, want 32", len(first))
	}
	if !bytes.Equal(first, second) {
		t.Fatal("MasterKey did not reuse the keyring value")
	}
}

func sampleProfile() Profile {
	return Profile{
		Name:               "work",
		Host:               "example.com",
		Browser:            "Chrome",
		BrowserProfile:     "Default",
		BrowserProfilePath: "/home/user/.config/google-chrome/Default",
		CreatedAt:          time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC),
		SyncedAt:           time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC),
		Cookies: []cookiemodel.Cookie{{
			Name: "session", Value: "super-secret", Domain: ".example.com", Path: "/",
		}},
	}
}
