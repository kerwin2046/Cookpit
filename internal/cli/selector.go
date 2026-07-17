package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cookiex/internal/chrome"
)

type SelectionStore interface {
	Load() (string, error)
	Save(profilePath string) error
}

type FileSelectionStore struct {
	path string
}

func NewFileSelectionStore(path string) *FileSelectionStore {
	return &FileSelectionStore{path: path}
}

func (s *FileSelectionStore) Load() (string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Chrome profile selection: %w", err)
	}
	var settings struct {
		ProfilePath string `json:"profile_path"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", fmt.Errorf("decode Chrome profile selection: %w", err)
	}
	return settings.ProfilePath, nil
}

func (s *FileSelectionStore) Save(profilePath string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create Cookiex config directory: %w", err)
	}
	data, err := json.Marshal(struct {
		ProfilePath string `json:"profile_path"`
	}{ProfilePath: profilePath})
	if err != nil {
		return fmt.Errorf("encode Chrome profile selection: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".selection-*")
	if err != nil {
		return fmt.Errorf("create Chrome profile selection: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("save Chrome profile selection: %w", err)
	}
	return nil
}

func SelectChromeProfile(
	profiles []chrome.Profile,
	explicit string,
	settings SelectionStore,
	input io.Reader,
	output io.Writer,
) (chrome.Profile, error) {
	if len(profiles) == 0 {
		return chrome.Profile{}, errors.New("no Chrome or Chromium profile with a Cookies database was found")
	}
	if explicit != "" {
		return findExplicitProfile(profiles, explicit)
	}

	if settings != nil {
		remembered, err := settings.Load()
		if err != nil {
			return chrome.Profile{}, err
		}
		for _, profile := range profiles {
			if profile.Path == remembered {
				return profile, nil
			}
		}
	}
	if len(profiles) == 1 {
		return profiles[0], nil
	}

	fmt.Fprintln(output, "Select a Chrome profile:")
	for index, profile := range profiles {
		fmt.Fprintf(output, "  %d. %s / %s\n", index+1, profile.Browser, profile.Name)
	}
	fmt.Fprint(output, "Profile: ")
	var selected int
	if _, err := fmt.Fscan(input, &selected); err != nil {
		return chrome.Profile{}, fmt.Errorf("read Chrome profile selection: %w", err)
	}
	if selected < 1 || selected > len(profiles) {
		return chrome.Profile{}, fmt.Errorf("profile selection %d is out of range", selected)
	}
	profile := profiles[selected-1]
	if settings != nil {
		if err := settings.Save(profile.Path); err != nil {
			return chrome.Profile{}, err
		}
	}
	return profile, nil
}

func findExplicitProfile(profiles []chrome.Profile, explicit string) (chrome.Profile, error) {
	var matches []chrome.Profile
	for _, profile := range profiles {
		if profile.Name == explicit || profile.Path == explicit ||
			profile.Browser+":"+profile.Name == explicit {
			matches = append(matches, profile)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return chrome.Profile{}, fmt.Errorf("Chrome profile %q is ambiguous; use Browser:Name or its full path", explicit)
	}
	return chrome.Profile{}, fmt.Errorf("Chrome profile %q was not found", explicit)
}
