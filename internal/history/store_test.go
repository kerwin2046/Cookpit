package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendHistoryDedupesAndCaps(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "playground.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := store.AppendHistory(Entry{
			SavedAt: now.Add(time.Duration(i) * time.Second),
			Profile: "work",
			Method:  "GET",
			URL:     "https://example.com/a",
			Headers: map[string]string{"Accept": "json", "Cookie": "secret"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AppendHistory(Entry{
		SavedAt: now.Add(3 * time.Second),
		Profile: "work",
		Method:  "POST",
		URL:     "https://example.com/a",
	}); err != nil {
		t.Fatal(err)
	}

	items := store.ListHistory()
	if len(items) != 2 {
		t.Fatalf("history len = %d, want 2", len(items))
	}
	if items[0].Method != "POST" || items[1].Method != "GET" {
		t.Fatalf("order = %#v", items)
	}
	if _, ok := items[1].Headers["Cookie"]; ok {
		t.Fatalf("cookie header leaked: %#v", items[1].Headers)
	}
	if items[1].Headers["Accept"] != "json" {
		t.Fatalf("headers = %#v", items[1].Headers)
	}
}

func TestPresetsUpsertListDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "playground.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePreset("me", Entry{
		Profile: "work",
		Method:  "GET",
		URL:     "https://example.com/me",
		Body:    "",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePreset("me", Entry{
		Profile: "work",
		Method:  "GET",
		URL:     "https://example.com/me2",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadPreset("me")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://example.com/me2" || got.Name != "me" {
		t.Fatalf("preset = %#v", got)
	}
	names := store.ListPresets()
	if len(names) != 1 || names[0].Name != "me" {
		t.Fatalf("list = %#v", names)
	}
	if err := store.DeletePreset("me"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPreset("me"); err == nil {
		t.Fatal("expected missing preset error")
	}
}

func TestOpenCreates0600File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playground.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendHistory(Entry{Profile: "p", Method: "GET", URL: "https://x/"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
}
