package cookie

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Cookie is the browser cookie data needed to decide whether it can be sent.
type Cookie struct {
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	Domain   string     `json:"domain"`
	Path     string     `json:"path"`
	Secure   bool       `json:"secure"`
	HTTPOnly bool       `json:"http_only"`
	SameSite int        `json:"same_site,omitempty"`
	HostOnly bool       `json:"host_only"`
	Expires  *time.Time `json:"expires,omitempty"`
}

// RequestContext contains the request properties used for cookie matching.
type RequestContext struct {
	Host   string
	Path   string
	Secure bool
	Now    time.Time
}

// NormalizeHost accepts a hostname, host:port, or URL and returns a canonical
// hostname suitable for cookie matching.
func NormalizeHost(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("host is empty")
	}

	parseTarget := input
	if !strings.Contains(parseTarget, "://") {
		parseTarget = "//" + parseTarget
	}

	parsed, err := url.Parse(parseTarget)
	if err != nil {
		return "", fmt.Errorf("parse host %q: %w", input, err)
	}

	host := parsed.Hostname()
	if host == "" {
		host = parsed.Path
		if parsedHost, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = parsedHost
		}
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || strings.ContainsAny(host, "/?#") {
		return "", fmt.Errorf("invalid host %q", input)
	}
	return host, nil
}

// Matches reports whether the cookie may be sent with the supplied request.
func (c Cookie) Matches(request RequestContext) bool {
	host := strings.Trim(strings.ToLower(request.Host), ".")
	domain := strings.Trim(strings.ToLower(c.Domain), ".")
	if host == "" || domain == "" {
		return false
	}

	if c.HostOnly {
		if host != domain {
			return false
		}
	} else if host != domain && !strings.HasSuffix(host, "."+domain) {
		return false
	}

	if c.Secure && !request.Secure {
		return false
	}

	cookiePath := c.Path
	if cookiePath == "" || cookiePath[0] != '/' {
		cookiePath = "/"
	}
	requestPath := request.Path
	if requestPath == "" || requestPath[0] != '/' {
		requestPath = "/"
	}
	if !pathMatches(cookiePath, requestPath) {
		return false
	}

	now := request.Now
	if now.IsZero() {
		now = time.Now()
	}
	return c.Expires == nil || c.Expires.After(now)
}

func pathMatches(cookiePath, requestPath string) bool {
	if cookiePath == requestPath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") ||
		(len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/')
}
