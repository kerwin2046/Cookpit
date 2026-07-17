package export

import (
	"strings"
	"testing"

	cookiemodel "cookiex/internal/cookie"
)

func TestRenderSupportedFormats(t *testing.T) {
	spec := RequestSpec{
		Method: "POST",
		URL:    "https://api.example.com/v1/items?q=hello world",
		Headers: map[string]string{
			"X-Test": "quoted \"value\"",
		},
		Body: `{"name":"it's ready"}`,
	}
	cookies := []cookiemodel.Cookie{
		{Name: "session", Value: "a'b", Domain: ".example.com", Path: "/", Secure: true},
		{Name: "wrong", Value: "no", Domain: ".other.com", Path: "/"},
	}

	tests := []struct {
		format   Format
		contains []string
	}{
		{
			format:   FormatCurl,
			contains: []string{"curl", "-X", "POST", "session=a'\"'\"'b", "--data-raw"},
		},
		{
			format:   FormatPython,
			contains: []string{"import requests", "requests.request(", `"Cookie": "session=a'b"`},
		},
		{
			format:   FormatJavaScript,
			contains: []string{"await fetch(", `"method": "POST"`, `"Cookie": "session=a'b"`},
		},
		{
			format:   FormatAxios,
			contains: []string{"axios(", `"method": "post"`, `"Cookie": "session=a'b"`},
		},
		{
			format:   FormatHTTPie,
			contains: []string{"http", "POST", "session=a'\"'\"'b", "X-Test:"},
		},
		{
			format:   FormatCurlCFFI,
			contains: []string{"from curl_cffi", "requests.request(", `"Cookie": "session=a'b"`, "impersonate"},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got, err := Render(tt.format, spec, cookies)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output does not contain %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "wrong=no") {
				t.Errorf("output contains cookie for another domain:\n%s", got)
			}
		})
	}
}

func TestParseSnippetFilterNoneAndAll(t *testing.T) {
	none, err := ParseSnippetFilter("none")
	if err != nil || none != nil {
		t.Fatalf("none = %v, %v", none, err)
	}
	empty, err := ParseSnippetFilter("")
	if err != nil || empty != nil {
		t.Fatalf("empty = %v, %v", empty, err)
	}
	all, err := ParseSnippetFilter("all")
	if err != nil || len(all) != len(SupportedFormats) {
		t.Fatalf("all = %v, %v", all, err)
	}
}

func TestRenderRejectsUnsupportedFormatAndInvalidURL(t *testing.T) {
	if _, err := Render(Format("go"), RequestSpec{URL: "https://example.com"}, nil); err == nil {
		t.Fatal("Render accepted unsupported format")
	}
	if _, err := Render(FormatCurl, RequestSpec{URL: "://bad"}, nil); err == nil {
		t.Fatal("Render accepted invalid URL")
	}
}
