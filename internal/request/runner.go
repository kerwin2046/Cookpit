package request

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"
)

const defaultMaxBodyBytes int64 = 10 << 20

type Spec struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

type Response struct {
	StatusCode int
	Status     string
	Headers    http.Header
	Body       []byte
	Truncated  bool
	Duration   time.Duration
}

type Runner struct {
	Client       *http.Client
	MaxBodyBytes int64
}

func (r Runner) Send(ctx context.Context, spec Spec, cookies []cookiemodel.Cookie) (Response, error) {
	parsed, err := ParseURL(spec.URL)
	if err != nil {
		return Response{}, err
	}
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(spec.Body))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}
	for name, value := range spec.Headers {
		request.Header.Set(name, value)
	}
	for _, item := range MatchingCookies(parsed, cookies, time.Now()) {
		request.AddCookie(&http.Cookie{Name: item.Name, Value: item.Value})
	}

	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	started := time.Now()
	httpResponse, err := client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	defer httpResponse.Body.Close()

	limit := r.MaxBodyBytes
	if limit <= 0 {
		limit = defaultMaxBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, limit+1))
	if err != nil {
		return Response{}, fmt.Errorf("read response body: %w", err)
	}
	truncated := int64(len(body)) > limit
	if truncated {
		body = body[:limit]
	}
	return Response{
		StatusCode: httpResponse.StatusCode,
		Status:     httpResponse.Status,
		Headers:    httpResponse.Header.Clone(),
		Body:       body,
		Truncated:  truncated,
		Duration:   time.Since(started),
	}, nil
}

func ParseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse request URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("request URL must use http or https and include a host")
	}
	return parsed, nil
}

func MatchingCookies(target *url.URL, cookies []cookiemodel.Cookie, now time.Time) []cookiemodel.Cookie {
	matched := make([]cookiemodel.Cookie, 0, len(cookies))
	for _, item := range cookies {
		if item.Matches(cookiemodel.RequestContext{
			Host:   target.Hostname(),
			Path:   target.EscapedPath(),
			Secure: target.Scheme == "https",
			Now:    now,
		}) {
			matched = append(matched, item)
		}
	}
	return matched
}

// MatchedCookieNames returns sorted cookie names that would be sent to target.
// Values are never included.
func MatchedCookieNames(target *url.URL, cookies []cookiemodel.Cookie, now time.Time) []string {
	matched := MatchingCookies(target, cookies, now)
	names := make([]string, len(matched))
	for i, item := range matched {
		names[i] = item.Name
	}
	sort.Strings(names)
	return names
}

// FormatMatchedCookiesLine builds a redacted Request-tab summary line.
func FormatMatchedCookiesLine(names []string) string {
	if len(names) == 0 {
		return "Cookie: [redacted — 0 matched]"
	}
	const maxNames = 12
	shown := names
	suffix := ""
	if len(names) > maxNames {
		shown = names[:maxNames]
		suffix = ", …"
	}
	return fmt.Sprintf("Cookie: [redacted — %d matched: %s%s]", len(names), strings.Join(shown, ", "), suffix)
}
