package tui

import (
	"net/http"
	"net/url"
	"strings"

	"cookiex/internal/history"
	hdrs "cookiex/internal/headers"
	requestmodel "cookiex/internal/request"
	"cookiex/internal/vault"
)

type HeaderRow struct {
	Name        string
	Value       string
	Enabled     bool
	FromProfile bool
}

func BuildSpec(method, url, body string, rows []HeaderRow) requestmodel.Spec {
	profileHeaders := make(map[string]string)
	requestHeaders := make(map[string]string)
	for _, row := range rows {
		if !row.Enabled || strings.TrimSpace(row.Name) == "" {
			continue
		}
		if row.FromProfile {
			profileHeaders[row.Name] = row.Value
		} else {
			requestHeaders[row.Name] = row.Value
		}
	}
	merged := hdrs.Merge(profileHeaders, requestHeaders)
	return requestmodel.Spec{
		Method:  orDefault(method, http.MethodGet),
		URL:     strings.TrimSpace(url),
		Headers: hdrs.Expand(merged, url),
		Body:    body,
	}
}

func HeadersFromProfile(profile vault.Profile) []HeaderRow {
	names := hdrs.SortedNames(profile.Headers)
	rows := make([]HeaderRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, HeaderRow{
			Name:        name,
			Value:       profile.Headers[name],
			Enabled:     true,
			FromProfile: true,
		})
	}
	return rows
}

func ProfileHeadersFromRows(rows []HeaderRow) map[string]string {
	out := make(map[string]string)
	for _, row := range rows {
		if !row.Enabled || !row.FromProfile || strings.TrimSpace(row.Name) == "" {
			continue
		}
		out[row.Name] = row.Value
	}
	return out
}

func HistoryEntryFromForm(profile, method, url, body string, rows []HeaderRow) history.Entry {
	headers := make(map[string]string)
	for _, row := range rows {
		if !row.Enabled || strings.TrimSpace(row.Name) == "" {
			continue
		}
		headers[row.Name] = row.Value
	}
	return history.Entry{
		Profile: profile,
		Method:  orDefault(method, http.MethodGet),
		URL:     strings.TrimSpace(url),
		Headers: headers,
		Body:    body,
	}
}

func ApplyHistoryHeaders(headers map[string]string) []HeaderRow {
	names := hdrs.SortedNames(headers)
	rows := make([]HeaderRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, HeaderRow{
			Name:        name,
			Value:       headers[name],
			Enabled:     true,
			FromProfile: false,
		})
	}
	return rows
}

// EnsureHostDerivedHeaders adds x-vis-domain={{host}} when the URL has a host
// and no x-vis-domain header is present yet.
func EnsureHostDerivedHeaders(rawURL string, rows []HeaderRow) []HeaderRow {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return rows
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Name), "x-vis-domain") {
			return rows
		}
	}
	return append(append([]HeaderRow(nil), rows...), HeaderRow{
		Name:        "x-vis-domain",
		Value:       "{{host}}",
		Enabled:     true,
		FromProfile: false,
	})
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
