package diff

import (
	"fmt"
	"sort"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

// Key identifies a cookie within a profile snapshot.
type Key struct {
	Name   string
	Domain string
	Path   string
}

func (k Key) String() string {
	return fmt.Sprintf("%s\t%s\t%s", k.Name, k.Domain, k.Path)
}

// Change describes how one cookie field differs between snapshot and live.
type Change struct {
	Field string
	From  string
	To    string
}

// ChangedCookie is a cookie present in both sides with differing attributes.
type ChangedCookie struct {
	Key     Key
	Before  cookiemodel.Cookie
	After   cookiemodel.Cookie
	Changes []Change
}

// Result is the set of differences between a snapshot and live cookies.
type Result struct {
	Added   []cookiemodel.Cookie
	Removed []cookiemodel.Cookie
	Changed []ChangedCookie
}

// Empty reports whether any differences were found.
func (r Result) Empty() bool {
	return len(r.Added) == 0 && len(r.Removed) == 0 && len(r.Changed) == 0
}

// Count returns the number of differing cookies.
func (r Result) Count() int {
	return len(r.Added) + len(r.Removed) + len(r.Changed)
}

// Compare returns cookies added, removed, or changed between snapshot and live.
// Identity is Name + Domain + Path.
func Compare(snapshot, live []cookiemodel.Cookie) Result {
	before := indexByKey(snapshot)
	after := indexByKey(live)

	result := Result{}
	for key, cookie := range after {
		if _, ok := before[key]; !ok {
			result.Added = append(result.Added, cookie)
		}
	}
	for key, cookie := range before {
		if _, ok := after[key]; !ok {
			result.Removed = append(result.Removed, cookie)
		}
	}
	for key, oldCookie := range before {
		newCookie, ok := after[key]
		if !ok {
			continue
		}
		changes := attributeChanges(oldCookie, newCookie)
		if len(changes) == 0 {
			continue
		}
		result.Changed = append(result.Changed, ChangedCookie{
			Key:     key,
			Before:  oldCookie,
			After:   newCookie,
			Changes: changes,
		})
	}

	sort.Slice(result.Added, func(i, j int) bool {
		return cookieLess(result.Added[i], result.Added[j])
	})
	sort.Slice(result.Removed, func(i, j int) bool {
		return cookieLess(result.Removed[i], result.Removed[j])
	})
	sort.Slice(result.Changed, func(i, j int) bool {
		return keyLess(result.Changed[i].Key, result.Changed[j].Key)
	})
	return result
}

// Format renders a human-readable diff. Values are redacted unless showValues.
func Format(profileName, host string, result Result, showValues bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Diff %s (%s)  snapshot → Chrome\n", profileName, host)
	if result.Empty() {
		b.WriteString("No differences.\n")
		return b.String()
	}

	for _, cookie := range result.Added {
		fmt.Fprintf(&b, "+ %s\n", formatCookieLine(cookie))
	}
	for _, cookie := range result.Removed {
		fmt.Fprintf(&b, "- %s\n", formatCookieLine(cookie))
	}
	for _, item := range result.Changed {
		fmt.Fprintf(&b, "~ %s  %s\n", formatCookieLine(item.After), formatChanges(item.Changes, showValues))
	}
	n := result.Count()
	if n == 1 {
		b.WriteString("1 difference\n")
	} else {
		fmt.Fprintf(&b, "%d differences\n", n)
	}
	return b.String()
}

func indexByKey(cookies []cookiemodel.Cookie) map[Key]cookiemodel.Cookie {
	indexed := make(map[Key]cookiemodel.Cookie, len(cookies))
	for _, cookie := range cookies {
		indexed[keyOf(cookie)] = cookie
	}
	return indexed
}

func keyOf(cookie cookiemodel.Cookie) Key {
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	return Key{Name: cookie.Name, Domain: cookie.Domain, Path: path}
}

func attributeChanges(before, after cookiemodel.Cookie) []Change {
	var changes []Change
	if before.Value != after.Value {
		changes = append(changes, Change{Field: "value", From: before.Value, To: after.Value})
	}
	if !expiresEqual(before.Expires, after.Expires) {
		changes = append(changes, Change{
			Field: "expires",
			From:  formatExpires(before.Expires),
			To:    formatExpires(after.Expires),
		})
	}
	if before.Secure != after.Secure {
		changes = append(changes, Change{
			Field: "secure",
			From:  formatBool(before.Secure),
			To:    formatBool(after.Secure),
		})
	}
	if before.HTTPOnly != after.HTTPOnly {
		changes = append(changes, Change{
			Field: "httponly",
			From:  formatBool(before.HTTPOnly),
			To:    formatBool(after.HTTPOnly),
		})
	}
	if before.SameSite != after.SameSite {
		changes = append(changes, Change{
			Field: "samesite",
			From:  fmt.Sprintf("%d", before.SameSite),
			To:    fmt.Sprintf("%d", after.SameSite),
		})
	}
	if before.HostOnly != after.HostOnly {
		changes = append(changes, Change{
			Field: "host_only",
			From:  formatBool(before.HostOnly),
			To:    formatBool(after.HostOnly),
		})
	}
	return changes
}

func expiresEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func formatExpires(value *time.Time) string {
	if value == nil {
		return "session"
	}
	return value.UTC().Format("2006-01-02")
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func formatCookieLine(cookie cookiemodel.Cookie) string {
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("%s\t%s\t%s", cookie.Name, cookie.Domain, path)
}

func formatChanges(changes []Change, showValues bool) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		from, to := change.From, change.To
		if change.Field == "value" && !showValues {
			parts = append(parts, "value changed")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s → %s", change.Field, from, to))
	}
	return strings.Join(parts, ", ")
}

func cookieLess(a, b cookiemodel.Cookie) bool {
	return keyLess(keyOf(a), keyOf(b))
}

func keyLess(a, b Key) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.Domain != b.Domain {
		return a.Domain < b.Domain
	}
	return a.Path < b.Path
}
