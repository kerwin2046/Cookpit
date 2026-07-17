package tui

import (
	"context"
	"net/http"
	"testing"
	"time"

	cookiemodel "cookiex/internal/cookie"
	requestmodel "cookiex/internal/request"
	"cookiex/internal/vault"
)

type memoryProfiles struct {
	profiles map[string]vault.Profile
}

func (m *memoryProfiles) Save(profile vault.Profile) error {
	if m.profiles == nil {
		m.profiles = map[string]vault.Profile{}
	}
	m.profiles[profile.Name] = profile
	return nil
}

func (m *memoryProfiles) Load(name string) (vault.Profile, error) {
	return m.profiles[name], nil
}

func (m *memoryProfiles) List() ([]string, error) {
	names := make([]string, 0, len(m.profiles))
	for name := range m.profiles {
		names = append(names, name)
	}
	return names, nil
}

type fakeRunner struct{}

func (fakeRunner) Send(context.Context, requestmodel.Spec, []cookiemodel.Cookie) (requestmodel.Response, error) {
	return requestmodel.Response{StatusCode: 200, Status: "200 OK", Duration: time.Millisecond}, nil
}

func TestNewLoadsProfileHeadersAndURL(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Headers: map[string]string{"x-vis-domain": "example.com"},
			Cookies: []cookiemodel.Cookie{{Name: "s", Value: "1", Domain: "example.com", Path: "/", HostOnly: true}},
		},
	}}
	model, err := New(Options{
		Profiles:    store,
		Runner:      fakeRunner{},
		ProfileName: "work",
		URL:         "https://example.com/api",
		Method:      http.MethodPost,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if model.urlInput.Value() != "https://example.com/api" {
		t.Fatalf("url = %q", model.urlInput.Value())
	}
	if model.methods[model.methodIdx] != "POST" {
		t.Fatalf("method = %s", model.methods[model.methodIdx])
	}
	if len(model.headers) != 1 || model.headers[0].Name != "x-vis-domain" || !model.headers[0].FromProfile {
		t.Fatalf("headers = %#v", model.headers)
	}
}

func TestNewRequiresProfiles(t *testing.T) {
	if _, err := New(Options{Profiles: &memoryProfiles{}}); err == nil {
		t.Fatal("expected error when no profiles exist")
	}
}
