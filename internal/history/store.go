package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxHistory = 50

// Entry is a saved playground request. Cookie header values are never persisted.
type Entry struct {
	ID      string            `json:"id,omitempty"`
	SavedAt time.Time         `json:"saved_at"`
	Profile string            `json:"profile"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Name    string            `json:"name,omitempty"`
}

type fileDoc struct {
	History []Entry `json:"history"`
	Presets []Entry `json:"presets"`
}

// Store persists playground history and named presets as JSON.
type Store struct {
	path string
	doc  fileDoc
}

// Open loads or creates a playground history file at path.
func Open(path string) (*Store, error) {
	store := &Store{path: path, doc: fileDoc{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read history: %w", err)
	}
	if len(raw) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(raw, &store.doc); err != nil {
		return nil, fmt.Errorf("decode history: %w", err)
	}
	if store.doc.History == nil {
		store.doc.History = nil
	}
	if store.doc.Presets == nil {
		store.doc.Presets = nil
	}
	return store, nil
}

// AppendHistory prepends an entry, dropping Cookie headers and consecutive duplicates.
func (s *Store) AppendHistory(entry Entry) error {
	entry.Name = ""
	entry.Headers = scrubHeaders(entry.Headers)
	if entry.SavedAt.IsZero() {
		entry.SavedAt = time.Now().UTC()
	}
	if len(s.doc.History) > 0 && sameRequest(s.doc.History[0], entry) {
		s.doc.History[0] = entry
		return s.save()
	}
	s.doc.History = append([]Entry{entry}, s.doc.History...)
	if len(s.doc.History) > maxHistory {
		s.doc.History = s.doc.History[:maxHistory]
	}
	return s.save()
}

// ListHistory returns history entries (newest first).
func (s *Store) ListHistory() []Entry {
	out := make([]Entry, len(s.doc.History))
	copy(out, s.doc.History)
	return out
}

// SavePreset upserts a named preset.
func (s *Store) SavePreset(name string, entry Entry) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("preset name is required")
	}
	entry.Name = name
	entry.Headers = scrubHeaders(entry.Headers)
	if entry.SavedAt.IsZero() {
		entry.SavedAt = time.Now().UTC()
	}
	for i, existing := range s.doc.Presets {
		if existing.Name == name {
			s.doc.Presets[i] = entry
			return s.save()
		}
	}
	s.doc.Presets = append(s.doc.Presets, entry)
	return s.save()
}

// ListPresets returns named presets in save order.
func (s *Store) ListPresets() []Entry {
	out := make([]Entry, 0, len(s.doc.Presets))
	for _, entry := range s.doc.Presets {
		if entry.Name != "" {
			out = append(out, entry)
		}
	}
	return out
}

// LoadPreset returns a preset by name.
func (s *Store) LoadPreset(name string) (Entry, error) {
	for _, entry := range s.doc.Presets {
		if entry.Name == name {
			return entry, nil
		}
	}
	return Entry{}, fmt.Errorf("preset %q not found", name)
}

// DeletePreset removes a named preset.
func (s *Store) DeletePreset(name string) error {
	for i, entry := range s.doc.Presets {
		if entry.Name == name {
			s.doc.Presets = append(s.doc.Presets[:i], s.doc.Presets[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("preset %q not found", name)
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	raw, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	raw = append(raw, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write history temp: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod history temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace history: %w", err)
	}
	return nil
}

func scrubHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		if strings.EqualFold(name, "Cookie") {
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sameRequest(a, b Entry) bool {
	return strings.EqualFold(a.Method, b.Method) && a.URL == b.URL && a.Profile == b.Profile
}
