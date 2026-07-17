package chrome

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

type staticSecretProvider struct {
	secret string
	err    error
}

func (p staticSecretProvider) Secret(string) (string, error) {
	return p.secret, p.err
}

type fakeChromeSecretBackend struct {
	secrets [][]byte
	locked  int
	err     error
}

func (b fakeChromeSecretBackend) Lookup(string) ([][]byte, int, error) {
	return b.secrets, b.locked, b.err
}

func TestLinuxDecrypterV10KnownVector(t *testing.T) {
	ciphertext, err := hex.DecodeString("763130a6b60a19719cd0fc909ca59e03ebf821")
	if err != nil {
		t.Fatal(err)
	}

	got, err := NewLinuxDecrypter("chrome", staticSecretProvider{}).Decrypt(ciphertext, "example.com", 23)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "session" {
		t.Fatalf("Decrypt = %q, want session", got)
	}
}

func TestLinuxDecrypterV11KnownVector(t *testing.T) {
	ciphertext, err := hex.DecodeString("76313196dd1e2a570208dca451a8396f9fbb89")
	if err != nil {
		t.Fatal(err)
	}

	got, err := NewLinuxDecrypter("chrome", staticSecretProvider{secret: "test-secret"}).Decrypt(ciphertext, "example.com", 23)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "profile" {
		t.Fatalf("Decrypt = %q, want profile", got)
	}
}

func TestLinuxDecrypterReportsUnavailableV11Secret(t *testing.T) {
	ciphertext, _ := hex.DecodeString("76313196dd1e2a570208dca451a8396f9fbb89")
	wantErr := errors.New("keyring locked")

	_, err := NewLinuxDecrypter("chrome", staticSecretProvider{err: wantErr}).Decrypt(ciphertext, "example.com", 23)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Decrypt error = %v, want wrapped %v", err, wantErr)
	}
}

func TestDecodePayloadVerifiesV24DomainHash(t *testing.T) {
	hash := sha256.Sum256([]byte(".example.com"))
	payload := append(hash[:], []byte("cookie-value")...)

	got, err := decodePayload(payload, ".example.com", 24)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if got != "cookie-value" {
		t.Fatalf("decodePayload = %q, want cookie-value", got)
	}

	if _, err := decodePayload(payload, ".wrong.example", 24); err == nil {
		t.Fatal("decodePayload accepted a mismatched domain hash")
	}
}

func TestLinuxDecrypterRejectsBadPadding(t *testing.T) {
	ciphertext, _ := hex.DecodeString("763130a6b60a19719cd0fc909ca59e03ebf820")
	if _, err := NewLinuxDecrypter("chrome", staticSecretProvider{}).Decrypt(ciphertext, "example.com", 23); err == nil {
		t.Fatal("Decrypt accepted invalid PKCS#7 padding")
	}
}

func TestChromeSecretProviderReturnsSingleUnlockedSecret(t *testing.T) {
	provider := newChromeSecretProvider(fakeChromeSecretBackend{secrets: [][]byte{[]byte("safe-storage")}})
	got, err := provider.Secret("chrome")
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if got != "safe-storage" {
		t.Fatalf("Secret = %q, want safe-storage", got)
	}
}

func TestChromeSecretProviderReportsLockedOrAmbiguousItems(t *testing.T) {
	tests := []struct {
		name    string
		backend fakeChromeSecretBackend
	}{
		{name: "locked", backend: fakeChromeSecretBackend{locked: 1}},
		{name: "missing", backend: fakeChromeSecretBackend{}},
		{name: "ambiguous", backend: fakeChromeSecretBackend{secrets: [][]byte{[]byte("a"), []byte("b")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newChromeSecretProvider(tt.backend).Secret("chrome"); err == nil {
				t.Fatal("Secret unexpectedly succeeded")
			}
		})
	}
}
