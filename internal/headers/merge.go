package headers

import (
	"sort"
	"strings"
)

// Merge combines profile default headers with request overrides.
// Request headers win on case-insensitive name collisions. Empty names are skipped.
func Merge(profileHeaders, requestHeaders map[string]string) map[string]string {
	merged := make(map[string]string)
	index := make(map[string]string) // lower -> canonical key used in output

	add := func(source map[string]string, override bool) {
		for name, value := range source {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if existing, ok := index[key]; ok {
				if override {
					delete(merged, existing)
					merged[name] = value
					index[key] = name
				}
				continue
			}
			merged[name] = value
			index[key] = name
		}
	}

	add(profileHeaders, false)
	add(requestHeaders, true)
	return merged
}

// SortedNames returns header names in stable order.
func SortedNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}
