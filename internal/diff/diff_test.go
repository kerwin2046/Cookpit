package diff

import (
	"strings"
	"testing"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

func TestCompareIdentical(t *testing.T) {
	cookies := []cookiemodel.Cookie{{
		Name: "session", Value: "tok", Domain: ".example.com", Path: "/",
		Secure: true, HTTPOnly: true,
	}}
	result := Compare(cookies, cookies)
	if !result.Empty() || result.Count() != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCompareAddedRemovedChanged(t *testing.T) {
	oldExpiry := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	newExpiry := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)

	snapshot := []cookiemodel.Cookie{
		{Name: "session", Value: "old", Domain: ".example.com", Path: "/", Expires: &oldExpiry},
		{Name: "old_token", Value: "gone", Domain: ".example.com", Path: "/"},
	}
	live := []cookiemodel.Cookie{
		{Name: "session", Value: "new", Domain: ".example.com", Path: "/", Expires: &newExpiry},
		{Name: "session2", Value: "fresh", Domain: ".example.com", Path: "/"},
	}

	result := Compare(snapshot, live)
	if result.Count() != 3 {
		t.Fatalf("count = %d, want 3: %#v", result.Count(), result)
	}
	if len(result.Added) != 1 || result.Added[0].Name != "session2" {
		t.Fatalf("added = %#v", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0].Name != "old_token" {
		t.Fatalf("removed = %#v", result.Removed)
	}
	if len(result.Changed) != 1 || result.Changed[0].Key.Name != "session" {
		t.Fatalf("changed = %#v", result.Changed)
	}
	fields := map[string]Change{}
	for _, change := range result.Changed[0].Changes {
		fields[change.Field] = change
	}
	if fields["value"].From != "old" || fields["value"].To != "new" {
		t.Fatalf("value change = %#v", fields["value"])
	}
	if fields["expires"].From != "2026-08-01" || fields["expires"].To != "2026-09-01" {
		t.Fatalf("expires change = %#v", fields["expires"])
	}
}

func TestCompareDetectsFlagChanges(t *testing.T) {
	snapshot := []cookiemodel.Cookie{{
		Name: "a", Value: "v", Domain: "example.com", Path: "/", HostOnly: true,
	}}
	live := []cookiemodel.Cookie{{
		Name: "a", Value: "v", Domain: "example.com", Path: "/",
		Secure: true, HTTPOnly: true, SameSite: 2, HostOnly: false,
	}}
	result := Compare(snapshot, live)
	if len(result.Changed) != 1 {
		t.Fatalf("changed = %#v", result.Changed)
	}
	got := map[string]bool{}
	for _, change := range result.Changed[0].Changes {
		got[change.Field] = true
	}
	for _, field := range []string{"secure", "httponly", "samesite", "host_only"} {
		if !got[field] {
			t.Fatalf("missing field %q in %#v", field, result.Changed[0].Changes)
		}
	}
}

func TestFormatRedactsValuesByDefault(t *testing.T) {
	oldExpiry := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	newExpiry := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	result := Result{
		Added:   []cookiemodel.Cookie{{Name: "session2", Domain: ".example.com", Path: "/", Value: "fresh-secret"}},
		Removed: []cookiemodel.Cookie{{Name: "old_token", Domain: ".example.com", Path: "/", Value: "gone-secret"}},
		Changed: []ChangedCookie{{
			Key:    Key{Name: "session", Domain: ".example.com", Path: "/"},
			Before: cookiemodel.Cookie{Name: "session", Value: "old-secret", Domain: ".example.com", Path: "/", Expires: &oldExpiry},
			After:  cookiemodel.Cookie{Name: "session", Value: "new-secret", Domain: ".example.com", Path: "/", Expires: &newExpiry},
			Changes: []Change{
				{Field: "value", From: "old-secret", To: "new-secret"},
				{Field: "expires", From: "2026-08-01", To: "2026-09-01"},
			},
		}},
	}

	output := Format("work", "example.com", result, false)
	for _, secret := range []string{"fresh-secret", "gone-secret", "old-secret", "new-secret"} {
		if strings.Contains(output, secret) {
			t.Fatalf("format leaked %q:\n%s", secret, output)
		}
	}
	for _, want := range []string{
		"Diff work (example.com)  snapshot → Chrome",
		"+ session2",
		"- old_token",
		"~ session",
		"value changed",
		"expires 2026-08-01 → 2026-09-01",
		"3 differences",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("format missing %q:\n%s", want, output)
		}
	}

	revealed := Format("work", "example.com", result, true)
	if !strings.Contains(revealed, "value old-secret → new-secret") {
		t.Fatalf("format --values missing secrets:\n%s", revealed)
	}
}

func TestFormatNoDifferences(t *testing.T) {
	output := Format("work", "example.com", Result{}, false)
	if !strings.Contains(output, "No differences.") {
		t.Fatalf("output = %q", output)
	}
}
