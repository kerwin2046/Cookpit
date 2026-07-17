package headers

import (
	"testing"
)

func TestMergeRequestOverridesProfile(t *testing.T) {
	got := Merge(
		map[string]string{
			"x-vis-domain": "www.compamed-tradefair.com",
			"Accept":       "application/json",
		},
		map[string]string{
			"Accept": "text/plain",
			"X-Test": "1",
		},
	)

	if got["x-vis-domain"] != "www.compamed-tradefair.com" {
		t.Fatalf("profile header lost: %#v", got)
	}
	if got["Accept"] != "text/plain" {
		t.Fatalf("request override failed: %#v", got)
	}
	if got["X-Test"] != "1" {
		t.Fatalf("request header missing: %#v", got)
	}
}

func TestMergeIsCaseInsensitiveForOverride(t *testing.T) {
	got := Merge(
		map[string]string{"Accept": "application/json"},
		map[string]string{"accept": "text/plain"},
	)
	if len(got) != 1 {
		t.Fatalf("expected one header after case-insensitive merge, got %#v", got)
	}
	if got["accept"] != "text/plain" && got["Accept"] != "text/plain" {
		t.Fatalf("override missing: %#v", got)
	}
}

func TestMergeSkipsEmptyNames(t *testing.T) {
	got := Merge(map[string]string{"": "x", "A": "1"}, nil)
	if _, ok := got[""]; ok || got["A"] != "1" {
		t.Fatalf("unexpected merge result: %#v", got)
	}
}
