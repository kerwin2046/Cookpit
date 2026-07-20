package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

func TestRunnerSendsOnlyMatchingCookies(t *testing.T) {
	var receivedCookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	parsed, _ := url.Parse(server.URL)
	cookies := []cookiemodel.Cookie{
		{Name: "session", Value: "yes", Domain: parsed.Hostname(), Path: "/", HostOnly: true},
		{Name: "path", Value: "yes", Domain: parsed.Hostname(), Path: "/api", HostOnly: true},
		{Name: "wrong", Value: "no", Domain: "example.com", Path: "/"},
		{Name: "secure", Value: "no", Domain: parsed.Hostname(), Path: "/", Secure: true},
	}

	runner := Runner{Client: server.Client(), MaxBodyBytes: 1024}
	response, err := runner.Send(context.Background(), Spec{
		Method: http.MethodGet,
		URL:    server.URL + "/api/items",
	}, cookies)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) != `{"ok":true}` {
		t.Fatalf("response = %#v", response)
	}
	if !strings.Contains(receivedCookie, "session=yes") || !strings.Contains(receivedCookie, "path=yes") {
		t.Fatalf("Cookie header = %q, want session and path", receivedCookie)
	}
	if strings.Contains(receivedCookie, "wrong=") || strings.Contains(receivedCookie, "secure=") {
		t.Fatalf("Cookie header includes non-matching cookie: %q", receivedCookie)
	}
}

func TestRunnerBoundsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "123456")
	}))
	defer server.Close()

	response, err := (Runner{Client: server.Client(), MaxBodyBytes: 4}).Send(
		context.Background(),
		Spec{Method: http.MethodGet, URL: server.URL},
		nil,
	)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if string(response.Body) != "1234" || !response.Truncated {
		t.Fatalf("bounded response = body %q truncated %v", response.Body, response.Truncated)
	}
}

func TestRunnerRejectsInvalidRequest(t *testing.T) {
	if _, err := (Runner{}).Send(context.Background(), Spec{Method: "BAD METHOD", URL: "://bad"}, nil); err == nil {
		t.Fatal("Send accepted an invalid request")
	}
}

func TestMatchedCookieNames(t *testing.T) {
	target, err := url.Parse("https://www.example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	cookies := []cookiemodel.Cookie{
		{Name: "session", Domain: ".example.com", Path: "/", Value: "secret"},
		{Name: "other", Domain: ".other.com", Path: "/", Value: "x"},
		{Name: "expired", Domain: ".example.com", Path: "/", Value: "x", Expires: &expired},
		{Name: "alpha", Domain: "www.example.com", Path: "/", Value: "a", HostOnly: true},
	}
	names := MatchedCookieNames(target, cookies, now)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "session" {
		t.Fatalf("names = %#v", names)
	}
}

func TestFormatMatchedCookiesLine(t *testing.T) {
	if got := FormatMatchedCookiesLine(nil); got != "Cookie: [redacted — 0 matched]" {
		t.Fatalf("empty = %q", got)
	}
	if got := FormatMatchedCookiesLine([]string{"a", "b"}); got != "Cookie: [redacted — 2 matched: a, b]" {
		t.Fatalf("two = %q", got)
	}
	many := make([]string, 13)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	got := FormatMatchedCookiesLine(many)
	if !strings.Contains(got, "13 matched:") || !strings.Contains(got, "…") {
		t.Fatalf("truncated = %q", got)
	}
}
