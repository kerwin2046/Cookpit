package headers

import "testing"

func TestExpandTemplates(t *testing.T) {
	got := Expand(map[string]string{
		"x-vis-domain": "{{host}}",
		"Origin":       "{{origin}}",
		"X-Scheme":     "{{scheme}}",
		"X-Keep":       "{{unknown}}",
		"Accept":       "application/json",
	}, "https://www.example.com/api")
	if got["x-vis-domain"] != "www.example.com" {
		t.Fatalf("host = %#v", got)
	}
	if got["Origin"] != "https://www.example.com" {
		t.Fatalf("origin = %#v", got)
	}
	if got["X-Scheme"] != "https" {
		t.Fatalf("scheme = %#v", got)
	}
	if got["X-Keep"] != "{{unknown}}" {
		t.Fatalf("unknown = %#v", got)
	}
	if got["Accept"] != "application/json" {
		t.Fatalf("literal = %#v", got)
	}
}

func TestExpandInvalidURLLeavesTemplates(t *testing.T) {
	got := Expand(map[string]string{"x-vis-domain": "{{host}}"}, "://bad")
	if got["x-vis-domain"] != "{{host}}" {
		t.Fatalf("got %#v", got)
	}
}
