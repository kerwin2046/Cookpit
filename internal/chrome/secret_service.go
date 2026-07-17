package chrome

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	secretServiceName = "org.freedesktop.secrets"
	secretServicePath = dbus.ObjectPath("/org/freedesktop/secrets")
)

type chromeSecretBackend interface {
	Lookup(application string) (secrets [][]byte, locked int, err error)
}

// ChromeSecretProvider retrieves the v11 Safe Storage password using the same
// "application" attribute Chrome and Chromium use in Secret Service.
type ChromeSecretProvider struct {
	backend chromeSecretBackend
}

func NewChromeSecretProvider() *ChromeSecretProvider {
	return newChromeSecretProvider(dbusChromeSecretBackend{})
}

func newChromeSecretProvider(backend chromeSecretBackend) *ChromeSecretProvider {
	return &ChromeSecretProvider{backend: backend}
}

func (p *ChromeSecretProvider) Secret(application string) (string, error) {
	secrets, locked, err := p.backend.Lookup(application)
	if err != nil {
		return "", err
	}
	if len(secrets) == 0 && locked > 0 {
		return "", fmt.Errorf("%s Safe Storage is locked in Secret Service", application)
	}
	if len(secrets) == 0 {
		return "", fmt.Errorf("%s Safe Storage was not found in Secret Service", application)
	}
	if len(secrets) != 1 {
		return "", fmt.Errorf("found %d unlocked %s Safe Storage entries; refusing an ambiguous key", len(secrets), application)
	}
	return string(secrets[0]), nil
}

type dbusChromeSecretBackend struct{}

func (dbusChromeSecretBackend) Lookup(application string) ([][]byte, int, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, 0, fmt.Errorf("connect to Secret Service session bus: %w", err)
	}
	defer conn.Close()

	service := conn.Object(secretServiceName, secretServicePath)
	var (
		output      dbus.Variant
		sessionPath dbus.ObjectPath
	)
	if err := service.Call(
		"org.freedesktop.Secret.Service.OpenSession",
		0,
		"plain",
		dbus.MakeVariant(""),
	).Store(&output, &sessionPath); err != nil {
		return nil, 0, fmt.Errorf("open Secret Service session: %w", err)
	}
	defer conn.Object(secretServiceName, sessionPath).
		Call("org.freedesktop.Secret.Session.Close", 0)

	var unlocked, locked []dbus.ObjectPath
	attributes := map[string]string{"application": application}
	if err := service.Call(
		"org.freedesktop.Secret.Service.SearchItems",
		0,
		attributes,
	).Store(&unlocked, &locked); err != nil {
		return nil, 0, fmt.Errorf("search %s Safe Storage: %w", application, err)
	}

	secrets := make([][]byte, 0, len(unlocked))
	for _, itemPath := range unlocked {
		var secret struct {
			Session     dbus.ObjectPath
			Parameters  []byte
			Value       []byte
			ContentType string
		}
		if err := conn.Object(secretServiceName, itemPath).Call(
			"org.freedesktop.Secret.Item.GetSecret",
			0,
			sessionPath,
		).Store(&secret); err != nil {
			return nil, len(locked), fmt.Errorf("read %s Safe Storage secret: %w", application, err)
		}
		secrets = append(secrets, append([]byte(nil), secret.Value...))
	}
	return secrets, len(locked), nil
}
