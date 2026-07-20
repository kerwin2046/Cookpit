package tui

import (
	"testing"

	"cookiex/internal/vault"
)

func TestBuildSpecMergesProfileAndRequestHeaders(t *testing.T) {
	spec := BuildSpec("POST", "https://example.com/api", `{"a":1}`, []HeaderRow{
		{Name: "x-vis-domain", Value: "example.com", Enabled: true, FromProfile: true},
		{Name: "Accept", Value: "application/json", Enabled: true, FromProfile: true},
		{Name: "Accept", Value: "text/plain", Enabled: true, FromProfile: false},
		{Name: "X-Skip", Value: "no", Enabled: false, FromProfile: false},
	})
	if spec.Method != "POST" || spec.URL != "https://example.com/api" || spec.Body != `{"a":1}` {
		t.Fatalf("spec basics = %#v", spec)
	}
	if spec.Headers["x-vis-domain"] != "example.com" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	if spec.Headers["Accept"] != "text/plain" {
		t.Fatalf("override failed: %#v", spec.Headers)
	}
	if _, ok := spec.Headers["X-Skip"]; ok {
		t.Fatalf("disabled header leaked: %#v", spec.Headers)
	}
}

func TestHeadersFromProfileAndSaveBack(t *testing.T) {
	profile := vault.Profile{Headers: map[string]string{
		"x-vis-domain": "www.compamed-tradefair.com",
		"Accept":       "application/json",
	}}
	rows := HeadersFromProfile(profile)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	rows = append(rows, HeaderRow{Name: "X-Temp", Value: "1", Enabled: true, FromProfile: false})
	saved := ProfileHeadersFromRows(rows)
	if saved["x-vis-domain"] != "www.compamed-tradefair.com" || saved["Accept"] != "application/json" {
		t.Fatalf("saved = %#v", saved)
	}
	if _, ok := saved["X-Temp"]; ok {
		t.Fatalf("request-only header saved as profile: %#v", saved)
	}
}

func TestApplyHistoryEntry(t *testing.T) {
	rows := ApplyHistoryHeaders(map[string]string{
		"Accept":       "application/json",
		"x-vis-domain": "example.com",
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	for _, row := range rows {
		if row.FromProfile || !row.Enabled {
			t.Fatalf("row = %#v", row)
		}
	}
}

func TestHistoryEntryFromForm(t *testing.T) {
	entry := HistoryEntryFromForm("work", "POST", "https://example.com/a", `{"a":1}`, []HeaderRow{
		{Name: "Accept", Value: "json", Enabled: true},
		{Name: "X-Skip", Value: "no", Enabled: false},
	})
	if entry.Profile != "work" || entry.Method != "POST" || entry.URL != "https://example.com/a" || entry.Body != `{"a":1}` {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.Headers["Accept"] != "json" {
		t.Fatalf("headers = %#v", entry.Headers)
	}
	if _, ok := entry.Headers["X-Skip"]; ok {
		t.Fatalf("disabled header included: %#v", entry.Headers)
	}
}

func TestEnsureHostDerivedHeaders(t *testing.T) {
	rows := EnsureHostDerivedHeaders("https://www.compamed-tradefair.com/vis-api/x", nil)
	if len(rows) != 1 || rows[0].Name != "x-vis-domain" || rows[0].Value != "{{host}}" || !rows[0].Enabled {
		t.Fatalf("rows = %#v", rows)
	}
	same := EnsureHostDerivedHeaders("https://www.compamed-tradefair.com/vis-api/x", rows)
	if len(same) != 1 {
		t.Fatalf("duplicate added: %#v", same)
	}
	empty := EnsureHostDerivedHeaders("not-a-url", nil)
	if len(empty) != 0 {
		t.Fatalf("invalid url = %#v", empty)
	}
}

func TestBuildSpecExpandsHostTemplate(t *testing.T) {
	spec := BuildSpec("GET", "https://www.example.com/api", "", []HeaderRow{
		{Name: "x-vis-domain", Value: "{{host}}", Enabled: true, FromProfile: true},
	})
	if spec.Headers["x-vis-domain"] != "www.example.com" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
}
