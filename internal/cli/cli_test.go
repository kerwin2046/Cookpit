package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"cookiex/internal/chrome"
	cookiemodel "cookiex/internal/cookie"
	requestmodel "cookiex/internal/request"
	"cookiex/internal/vault"
)

type memoryProfiles struct {
	profiles map[string]vault.Profile
}

func (m *memoryProfiles) Save(profile vault.Profile) error {
	if m.profiles == nil {
		m.profiles = make(map[string]vault.Profile)
	}
	m.profiles[profile.Name] = profile
	return nil
}

func (m *memoryProfiles) Load(name string) (vault.Profile, error) {
	profile, ok := m.profiles[name]
	if !ok {
		return vault.Profile{}, os.ErrNotExist
	}
	return profile, nil
}

func (m *memoryProfiles) Exists(name string) (bool, error) {
	_, ok := m.profiles[name]
	return ok, nil
}

func (m *memoryProfiles) List() ([]string, error) {
	names := make([]string, 0, len(m.profiles))
	for name := range m.profiles {
		names = append(names, name)
	}
	return names, nil
}

type fakeRunner struct {
	spec    requestmodel.Spec
	cookies []cookiemodel.Cookie
}

func (r *fakeRunner) Send(_ context.Context, spec requestmodel.Spec, cookies []cookiemodel.Cookie) (requestmodel.Response, error) {
	r.spec = spec
	r.cookies = cookies
	return requestmodel.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"ok":true}`),
		Duration:   time.Millisecond,
	}, nil
}

func TestImportProfilesExportSendAndSync(t *testing.T) {
	store := &memoryProfiles{}
	runner := &fakeRunner{}
	now := time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC)
	readCount := 0
	services := Services{
		ConfigHome: "/config",
		Profiles:   store,
		DiscoverProfiles: func(string) ([]chrome.Profile, error) {
			return []chrome.Profile{{
				Browser: "Chrome", Application: "chrome", Name: "Default",
				Path: "/config/google-chrome/Default", CookiesPath: "/cookies",
			}}, nil
		},
		ReadCookies: func(_ context.Context, _ chrome.Profile, host string) ([]cookiemodel.Cookie, error) {
			readCount++
			return []cookiemodel.Cookie{{
				Name: "session", Value: "super-secret", Domain: host, Path: "/", HostOnly: true,
			}}, nil
		},
		Runner: runner,
		Now:    func() time.Time { return now },
	}

	importOutput := execute(t, services, "import", "example.com", "--profile", "work")
	if strings.Contains(importOutput, "super-secret") {
		t.Fatal("import output exposed cookie value")
	}
	saved := store.profiles["work"]
	if saved.Host != "example.com" || len(saved.Cookies) != 1 {
		t.Fatalf("saved profile = %#v", saved)
	}

	profilesOutput := execute(t, services, "profiles")
	if !strings.Contains(profilesOutput, "work") || strings.Contains(profilesOutput, "super-secret") {
		t.Fatalf("profiles output = %q", profilesOutput)
	}

	exportOutput := execute(t, services, "export", "work", "--format", "curl")
	if !strings.Contains(exportOutput, "session=super-secret") {
		t.Fatalf("export output = %q", exportOutput)
	}

	sendOutput := execute(t, services, "send", "GET", "https://example.com/account", "--profile", "work")
	if !strings.Contains(sendOutput, "200 OK") || !strings.Contains(sendOutput, `"ok": true`) {
		t.Fatalf("send output = %q", sendOutput)
	}
	if len(runner.cookies) != 1 || runner.cookies[0].Value != "super-secret" {
		t.Fatalf("runner cookies = %#v", runner.cookies)
	}

	now = now.Add(time.Hour)
	syncOutput := execute(t, services, "sync", "work")
	if !strings.Contains(syncOutput, "Synced") || readCount != 2 {
		t.Fatalf("sync output = %q, read count = %d", syncOutput, readCount)
	}
	if !store.profiles["work"].CreatedAt.Equal(saved.CreatedAt) ||
		!store.profiles["work"].SyncedAt.Equal(now) {
		t.Fatalf("synced profile timestamps = %#v", store.profiles["work"])
	}
}

func TestImportRefusesOverwriteWithoutForce(t *testing.T) {
	store := &memoryProfiles{}
	services := Services{
		ConfigHome: "/config",
		Profiles:   store,
		DiscoverProfiles: func(string) ([]chrome.Profile, error) {
			return []chrome.Profile{{
				Browser: "Chrome", Application: "chrome", Name: "Default",
				Path: "/config/google-chrome/Default", CookiesPath: "/cookies",
			}}, nil
		},
		ReadCookies: func(_ context.Context, _ chrome.Profile, host string) ([]cookiemodel.Cookie, error) {
			return []cookiemodel.Cookie{{
				Name: "session", Value: "token-" + host, Domain: host, Path: "/", HostOnly: true,
			}}, nil
		},
		Now: func() time.Time { return time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC) },
	}

	execute(t, services, "import", "example.com", "--profile", "work")
	err := executeError(t, services, "import", "other.com", "--profile", "work")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("overwrite error = %v, want mention of --force", err)
	}
	if store.profiles["work"].Host != "example.com" {
		t.Fatalf("profile was overwritten: %#v", store.profiles["work"])
	}

	output := execute(t, services, "import", "other.com", "--profile", "work", "--force")
	if !strings.Contains(output, "other.com") {
		t.Fatalf("forced import output = %q", output)
	}
	if store.profiles["work"].Host != "other.com" {
		t.Fatalf("forced profile = %#v", store.profiles["work"])
	}
}

func TestShowListsCookiesWithoutValues(t *testing.T) {
	expires := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com", Browser: "Chrome", BrowserProfile: "Default",
			SyncedAt: time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC),
			Cookies: []cookiemodel.Cookie{{
				Name: "session", Value: "super-secret", Domain: ".example.com", Path: "/",
				Secure: true, HTTPOnly: true, Expires: &expires,
			}},
		},
	}}
	services := Services{Profiles: store}

	output := execute(t, services, "show", "work")
	if strings.Contains(output, "super-secret") {
		t.Fatal("show exposed cookie value")
	}
	for _, want := range []string{"session", ".example.com", "/", "secure", "httponly", "2027-01-01"} {
		if !strings.Contains(output, want) {
			t.Fatalf("show output missing %q:\n%s", want, output)
		}
	}

	revealed := execute(t, services, "show", "work", "--values")
	if !strings.Contains(revealed, "super-secret") {
		t.Fatalf("show --values missing secret:\n%s", revealed)
	}
}

func TestPlaySendsAndShowsResponseWithCodeSnippets(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{{
				Name: "session", Value: "super-secret", Domain: "example.com", Path: "/", HostOnly: true,
			}},
		},
	}}
	runner := &fakeRunner{}
	services := Services{Profiles: store, Runner: runner}

	output := execute(t, services, "play", "https://example.com/api/me", "--profile", "work")
	if !strings.Contains(output, "200 OK") || !strings.Contains(output, `"ok": true`) {
		t.Fatalf("play missing response viewer:\n%s", output)
	}
	if !strings.Contains(output, "Response") {
		t.Fatalf("play missing Response section:\n%s", output)
	}
	if !strings.Contains(output, "curl") || !strings.Contains(output, "session=super-secret") {
		t.Fatalf("play missing curl snippet:\n%s", output)
	}
	if strings.Contains(output, "await fetch(") {
		t.Fatalf("play default should not dump all snippets:\n%s", output)
	}
	if len(runner.cookies) != 1 || runner.cookies[0].Value != "super-secret" {
		t.Fatalf("play did not send cookies: %#v", runner.cookies)
	}
	if runner.spec.URL != "https://example.com/api/me" {
		t.Fatalf("play URL = %q", runner.spec.URL)
	}
}

func TestPlaySupportsMethodHeadersBodyAndSnippetFilter(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{{
				Name: "session", Value: "tok", Domain: "example.com", Path: "/", HostOnly: true,
			}},
		},
	}}
	runner := &fakeRunner{}
	services := Services{Profiles: store, Runner: runner}

	output := execute(t, services,
		"play", "https://example.com/api/items",
		"--profile", "work",
		"-X", "POST",
		"-H", "Content-Type=application/json",
		"-d", `{"name":"demo"}`,
		"--snippet", "python",
	)
	if runner.spec.Method != "POST" || runner.spec.Body != `{"name":"demo"}` {
		t.Fatalf("play request = %#v", runner.spec)
	}
	if runner.spec.Headers["Content-Type"] != "application/json" {
		t.Fatalf("play headers = %#v", runner.spec.Headers)
	}
	if !strings.Contains(output, "import requests") {
		t.Fatalf("play --snippet python missing requests:\n%s", output)
	}
	if strings.Contains(output, "await fetch(") || strings.Contains(output, "curl -X") {
		t.Fatalf("play --snippet python leaked other formats:\n%s", output)
	}
}

func TestPlayUsesProfileHeadersAndAllowsOverride(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Headers: map[string]string{"x-vis-domain": "example.com", "Accept": "application/json"},
			Cookies: []cookiemodel.Cookie{{
				Name: "session", Value: "tok", Domain: "example.com", Path: "/", HostOnly: true,
			}},
		},
	}}
	runner := &fakeRunner{}
	services := Services{Profiles: store, Runner: runner}

	execute(t, services, "play", "https://example.com/api", "--profile", "work", "--snippet", "none", "-H", "Accept=text/plain")
	if runner.spec.Headers["x-vis-domain"] != "example.com" {
		t.Fatalf("profile header missing: %#v", runner.spec.Headers)
	}
	if runner.spec.Headers["Accept"] != "text/plain" {
		t.Fatalf("override failed: %#v", runner.spec.Headers)
	}
}

func TestPlayShowsAllClientSnippets(t *testing.T) {
	store := &memoryProfiles{profiles: map[string]vault.Profile{
		"work": {
			Name: "work", Host: "example.com",
			Cookies: []cookiemodel.Cookie{{
				Name: "session", Value: "tok", Domain: "example.com", Path: "/", HostOnly: true,
			}},
		},
	}}
	services := Services{Profiles: store, Runner: &fakeRunner{}}

	output := execute(t, services, "play", "https://example.com/", "--profile", "work", "--snippet", "all")
	for _, want := range []string{"curl", "requests", "fetch", "axios", "http ", "curl_cffi"} {
		if !strings.Contains(output, want) {
			t.Fatalf("play --snippet all missing %q:\n%s", want, output)
		}
	}
}

func TestSelectorPromptsOnceAndRemembersChoice(t *testing.T) {
	settings := NewFileSelectionStore(t.TempDir() + "/selection.json")
	profiles := []chrome.Profile{
		{Browser: "Chrome", Name: "Default", Path: "/chrome/Default"},
		{Browser: "Chrome", Name: "Profile 2", Path: "/chrome/Profile 2"},
	}

	var output bytes.Buffer
	selected, err := SelectChromeProfile(profiles, "", settings, strings.NewReader("2\n"), &output)
	if err != nil {
		t.Fatalf("first selection: %v", err)
	}
	if selected.Name != "Profile 2" {
		t.Fatalf("selected = %#v", selected)
	}

	output.Reset()
	selected, err = SelectChromeProfile(profiles, "", settings, strings.NewReader(""), &output)
	if err != nil {
		t.Fatalf("remembered selection: %v", err)
	}
	if selected.Name != "Profile 2" || output.Len() != 0 {
		t.Fatalf("remembered selection = %#v, output=%q", selected, output.String())
	}
}

func execute(t *testing.T, services Services, args ...string) string {
	t.Helper()
	output, err := runCommand(services, args...)
	if err != nil {
		t.Fatalf("execute %v: %v\n%s", args, err, output)
	}
	return output
}

func executeError(t *testing.T, services Services, args ...string) error {
	t.Helper()
	_, err := runCommand(services, args...)
	return err
}

func runCommand(services Services, args ...string) (string, error) {
	var output bytes.Buffer
	command := NewRootCommand(services)
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}
