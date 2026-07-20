package tui

import (
	"context"
	"net/http"
	"testing"
	"time"

	cookiemodel "cookiex/internal/cookie"
	historypkg "cookiex/internal/history"
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

type fakeSyncer struct {
	profile vault.Profile
	err     error
}

func (f fakeSyncer) Sync(context.Context, vault.Profile) (vault.Profile, error) {
	return f.profile, f.err
}

func TestSyncDoneUpdatesCookies(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{{Name: "old", Value: "1", Domain: "example.com", Path: "/", HostOnly: true}},
		},
	}}
	model, err := New(Options{
		Profiles:    store,
		Runner:      fakeRunner{},
		Syncer:      fakeSyncer{},
		ProfileName: "work",
		URL:         "https://example.com/",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, cmd := model.Update(syncDoneMsg{
		oldCount: 1,
		profile: vault.Profile{
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{
				{Name: "fresh", Value: "2", Domain: "example.com", Path: "/", HostOnly: true},
				{Name: "extra", Value: "3", Domain: "example.com", Path: "/", HostOnly: true},
			},
		},
	})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	m := updated.(*Model)
	if len(m.profile.Cookies) != 2 || m.profile.Cookies[0].Name != "fresh" {
		t.Fatalf("cookies = %#v", m.profile.Cookies)
	}
	if m.status != "synced 1 → 2 cookies" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestMatchedCookiesLineUsesURL(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{
				{Name: "session", Value: "1", Domain: "example.com", Path: "/", HostOnly: true},
				{Name: "other", Value: "1", Domain: "other.com", Path: "/"},
			},
		},
	}}
	model, err := New(Options{Profiles: store, Runner: fakeRunner{}, ProfileName: "work", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	line := model.matchedCookiesLine("https://example.com/a")
	if line != "Cookie: [redacted — 1 matched: session]" {
		t.Fatalf("line = %q", line)
	}
}

type memoryHistory struct {
	history []historypkg.Entry
	presets []historypkg.Entry
}

func (m *memoryHistory) AppendHistory(entry historypkg.Entry) error {
	m.history = append([]historypkg.Entry{entry}, m.history...)
	return nil
}

func (m *memoryHistory) ListHistory() []historypkg.Entry { return m.history }

func (m *memoryHistory) SavePreset(name string, entry historypkg.Entry) error {
	entry.Name = name
	for i, existing := range m.presets {
		if existing.Name == name {
			m.presets[i] = entry
			return nil
		}
	}
	m.presets = append(m.presets, entry)
	return nil
}

func (m *memoryHistory) ListPresets() []historypkg.Entry { return m.presets }

func (m *memoryHistory) LoadPreset(name string) (historypkg.Entry, error) {
	for _, entry := range m.presets {
		if entry.Name == name {
			return entry, nil
		}
	}
	return historypkg.Entry{}, context.Canceled
}

func TestCycleHistoryAppliesEntry(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {Name: "work", Host: "example.com"},
	}}
	hist := &memoryHistory{history: []historypkg.Entry{{
		Profile: "work", Method: "POST", URL: "https://example.com/items", Body: `{}`,
		Headers: map[string]string{"Accept": "json"},
	}}}
	model, err := New(Options{
		Profiles: store, Runner: fakeRunner{}, History: hist,
		ProfileName: "work", URL: "https://example.com/",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	model.cycleHistory(1)
	if model.urlInput.Value() != "https://example.com/items" || model.methods[model.methodIdx] != "POST" {
		t.Fatalf("form = url %q method %s", model.urlInput.Value(), model.methods[model.methodIdx])
	}
	if len(model.headers) != 2 {
		t.Fatalf("headers = %#v", model.headers)
	}
	foundAccept, foundDomain := false, false
	for _, row := range model.headers {
		if row.Name == "Accept" && row.Value == "json" {
			foundAccept = true
		}
		if row.Name == "x-vis-domain" && row.Value == "{{host}}" {
			foundDomain = true
		}
	}
	if !foundAccept || !foundDomain {
		t.Fatalf("headers = %#v", model.headers)
	}
}
