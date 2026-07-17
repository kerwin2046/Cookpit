package chrome

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Profile identifies a Chrome-family profile and its cookie database.
type Profile struct {
	Browser     string `json:"browser"`
	Application string `json:"application"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	CookiesPath string `json:"cookies_path"`
}

// DiscoverProfiles finds Chrome and Chromium profiles under a Linux config
// directory, normally $XDG_CONFIG_HOME or ~/.config.
func DiscoverProfiles(configHome string) ([]Profile, error) {
	browsers := []struct {
		dir         string
		name        string
		application string
	}{
		{dir: "google-chrome", name: "Chrome", application: "chrome"},
		{dir: "chromium", name: "Chromium", application: "chromium"},
	}

	var profiles []Profile
	for _, browser := range browsers {
		root := filepath.Join(configHome, browser.dir)
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s profiles: %w", browser.name, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			profilePath := filepath.Join(root, entry.Name())
			cookiesPath := findCookiesPath(profilePath)
			if cookiesPath == "" {
				continue
			}
			profiles = append(profiles, Profile{
				Browser:     browser.name,
				Application: browser.application,
				Name:        entry.Name(),
				Path:        profilePath,
				CookiesPath: cookiesPath,
			})
		}
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Browser != profiles[j].Browser {
			return profiles[i].Browser < profiles[j].Browser
		}
		return profileSortKey(profiles[i].Name) < profileSortKey(profiles[j].Name)
	})
	return profiles, nil
}

func findCookiesPath(profilePath string) string {
	for _, relative := range []string{filepath.Join("Network", "Cookies"), "Cookies"} {
		path := filepath.Join(profilePath, relative)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func profileSortKey(name string) string {
	if name == "Default" {
		return "\x00"
	}
	return name
}
