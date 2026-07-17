package chrome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProfiles(t *testing.T) {
	configHome := t.TempDir()
	createCookieDBFile(t, filepath.Join(configHome, "google-chrome", "Default", "Network", "Cookies"))
	createCookieDBFile(t, filepath.Join(configHome, "google-chrome", "Profile 2", "Cookies"))
	createCookieDBFile(t, filepath.Join(configHome, "chromium", "Default", "Network", "Cookies"))
	createCookieDBFile(t, filepath.Join(configHome, "unrelated", "Default", "Network", "Cookies"))

	profiles, err := DiscoverProfiles(configHome)
	if err != nil {
		t.Fatalf("DiscoverProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3: %#v", len(profiles), profiles)
	}

	if profiles[0].Browser != "Chrome" || profiles[0].Name != "Default" {
		t.Errorf("first profile = %#v, want Chrome Default", profiles[0])
	}
	if profiles[1].Name != "Profile 2" {
		t.Errorf("second profile = %#v, want Chrome Profile 2", profiles[1])
	}
	if profiles[2].Browser != "Chromium" {
		t.Errorf("third profile = %#v, want Chromium", profiles[2])
	}
}

func createCookieDBFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
