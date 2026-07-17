package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
