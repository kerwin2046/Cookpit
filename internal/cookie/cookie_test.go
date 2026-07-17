package cookie

import (
	"testing"
	"time"
)

func TestNormalizeHost(t *testing.T) {
	tests := map[string]string{
		"github.com":                    "github.com",
		"HTTPS://Sub.GitHub.com/path?q": "sub.github.com",
		"sub.github.com:8443":           "sub.github.com",
		"  .Example.COM.  ":             "example.com",
	}

	for input, want := range tests {
		got, err := NormalizeHost(input)
		if err != nil {
			t.Fatalf("NormalizeHost(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCookieMatchesRequest(t *testing.T) {
	now := time.Date(2026, time.July, 17, 7, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name    string
		cookie  Cookie
		request RequestContext
		want    bool
	}{
		{
			name:    "domain cookie matches subdomain",
			cookie:  Cookie{Domain: ".example.com", Path: "/", Expires: &future},
			request: RequestContext{Host: "api.example.com", Path: "/", Secure: true, Now: now},
			want:    true,
		},
		{
			name:    "domain suffix confusion is rejected",
			cookie:  Cookie{Domain: ".example.com", Path: "/"},
			request: RequestContext{Host: "badexample.com", Path: "/", Now: now},
			want:    false,
		},
		{
			name:    "host-only cookie rejects subdomain",
			cookie:  Cookie{Domain: "example.com", HostOnly: true, Path: "/"},
			request: RequestContext{Host: "api.example.com", Path: "/", Now: now},
			want:    false,
		},
		{
			name:    "secure cookie rejects http",
			cookie:  Cookie{Domain: "example.com", Path: "/", Secure: true},
			request: RequestContext{Host: "example.com", Path: "/", Secure: false, Now: now},
			want:    false,
		},
		{
			name:    "cookie path matches child path",
			cookie:  Cookie{Domain: "example.com", Path: "/account"},
			request: RequestContext{Host: "example.com", Path: "/account/settings", Now: now},
			want:    true,
		},
		{
			name:    "cookie path requires segment boundary",
			cookie:  Cookie{Domain: "example.com", Path: "/account"},
			request: RequestContext{Host: "example.com", Path: "/accounting", Now: now},
			want:    false,
		},
		{
			name:    "expired cookie is rejected",
			cookie:  Cookie{Domain: "example.com", Path: "/", Expires: &past},
			request: RequestContext{Host: "example.com", Path: "/", Now: now},
			want:    false,
		},
		{
			name:    "session cookie is accepted",
			cookie:  Cookie{Domain: "example.com", Path: "/"},
			request: RequestContext{Host: "example.com", Path: "/", Now: now},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cookie.Matches(tt.request); got != tt.want {
				t.Fatalf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
