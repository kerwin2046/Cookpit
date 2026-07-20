package tui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	requestmodel "cookiex/internal/request"
)

func TestFormatResponseHeaders(t *testing.T) {
	resp := &requestmodel.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Duration:   824 * time.Millisecond,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
	}
	got := FormatResponseHeaders(resp)
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "824ms") {
		t.Fatalf("status missing: %q", got)
	}
	if !strings.Contains(got, "Content-Type: application/json") {
		t.Fatalf("header missing: %q", got)
	}
	if strings.Contains(got, "{") {
		t.Fatalf("body leaked into headers view: %q", got)
	}
}

func TestFormatResponseBodyJSON(t *testing.T) {
	got := FormatResponseBody([]byte(`{"a":1}`), false)
	if got != "{\n  \"a\": 1\n}" {
		t.Fatalf("body = %q", got)
	}
}

func TestFormatResponseBodyEmpty(t *testing.T) {
	if got := FormatResponseBody(nil, false); got != "(empty body)" {
		t.Fatalf("empty = %q", got)
	}
}

func TestCopyTargetLabel(t *testing.T) {
	if CopyTargetLabel(tabResponse, respViewBody, 0) != "response body" {
		t.Fatal("body label")
	}
	if CopyTargetLabel(tabResponse, respViewHeaders, 0) != "response headers" {
		t.Fatal("headers label")
	}
	if CopyTargetLabel(tabCode, respViewBody, 0) != "curl" {
		t.Fatal("code label")
	}
}
