package headers

import (
	"net/url"
	"strings"
)

// Expand replaces {{host}}, {{origin}}, and {{scheme}} in header values using rawURL.
// Unknown placeholders are left unchanged. Invalid URLs leave values unchanged.
func Expand(headers map[string]string, rawURL string) map[string]string {
	if len(headers) == 0 {
		return headers
	}
	host, origin, scheme := "", "", ""
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
		scheme = parsed.Scheme
		if scheme != "" {
			origin = scheme + "://" + host
		}
	}
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		out[name] = expandValue(value, host, origin, scheme)
	}
	return out
}

func expandValue(value, host, origin, scheme string) string {
	if !strings.Contains(value, "{{") {
		return value
	}
	if host == "" {
		return value
	}
	replacer := strings.NewReplacer(
		"{{host}}", host,
		"{{origin}}", origin,
		"{{scheme}}", scheme,
	)
	return replacer.Replace(value)
}
