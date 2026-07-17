package export

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"
	requestmodel "cookiex/internal/request"
)

type Format string

const (
	FormatCurl       Format = "curl"
	FormatPython     Format = "python"
	FormatJavaScript Format = "javascript"
	FormatAxios      Format = "axios"
	FormatHTTPie     Format = "httpie"
	FormatCurlCFFI   Format = "curl_cffi"
)

type RequestSpec = requestmodel.Spec

// SupportedFormats is the default playground snippet order.
var SupportedFormats = []Format{
	FormatCurl,
	FormatPython,
	FormatJavaScript,
	FormatAxios,
	FormatHTTPie,
	FormatCurlCFFI,
}

func Render(format Format, spec RequestSpec, cookies []cookiemodel.Cookie) (string, error) {
	target, err := requestmodel.ParseURL(spec.URL)
	if err != nil {
		return "", err
	}
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodGet
	}

	headers := make(map[string]string, len(spec.Headers)+1)
	for name, value := range spec.Headers {
		headers[name] = value
	}
	matched := requestmodel.MatchingCookies(target, cookies, time.Now())
	if len(matched) > 0 {
		headers["Cookie"] = cookieHeader(matched)
	}

	switch format {
	case FormatCurl:
		return renderCurl(method, target.String(), headers, spec.Body), nil
	case FormatPython:
		return renderPython(method, target.String(), headers, spec.Body)
	case FormatJavaScript:
		return renderJavaScript(method, target.String(), headers, spec.Body)
	case FormatAxios:
		return renderAxios(method, target.String(), headers, spec.Body)
	case FormatHTTPie:
		return renderHTTPie(method, target.String(), headers, spec.Body), nil
	case FormatCurlCFFI:
		return renderCurlCFFI(method, target.String(), headers, spec.Body)
	default:
		return "", fmt.Errorf("unsupported export format %q (use curl, python, javascript, axios, httpie, or curl_cffi)", format)
	}
}

// ParseSnippetFilter returns the formats to render for play/export.
// "all" means every supported format. Empty or "none" means no snippets.
func ParseSnippetFilter(raw string) ([]Format, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "none" {
		return nil, nil
	}
	if raw == "all" {
		return append([]Format(nil), SupportedFormats...), nil
	}
	aliases := map[string]Format{
		"curl":       FormatCurl,
		"python":     FormatPython,
		"requests":   FormatPython,
		"javascript": FormatJavaScript,
		"js":         FormatJavaScript,
		"fetch":      FormatJavaScript,
		"axios":      FormatAxios,
		"httpie":     FormatHTTPie,
		"http":       FormatHTTPie,
		"curl_cffi":  FormatCurlCFFI,
		"curl-cffi":  FormatCurlCFFI,
	}
	parts := strings.Split(raw, ",")
	formats := make([]Format, 0, len(parts))
	seen := make(map[Format]bool)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		format, ok := aliases[part]
		if !ok {
			return nil, fmt.Errorf("unknown snippet %q (use curl, python, javascript, axios, httpie, curl_cffi, all, or none)", part)
		}
		if seen[format] {
			continue
		}
		seen[format] = true
		formats = append(formats, format)
	}
	return formats, nil
}

func FormatLabel(format Format) string {
	switch format {
	case FormatPython:
		return "Python requests"
	case FormatJavaScript:
		return "JavaScript fetch"
	case FormatCurlCFFI:
		return "Python curl_cffi"
	case FormatHTTPie:
		return "HTTPie"
	case FormatAxios:
		return "JavaScript axios"
	default:
		return string(format)
	}
}

func cookieHeader(cookies []cookiemodel.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, item := range cookies {
		parts = append(parts, item.Name+"="+item.Value)
	}
	return strings.Join(parts, "; ")
}

func renderCurl(method, target string, headers map[string]string, body string) string {
	parts := []string{"curl", "-X", shellQuote(method), shellQuote(target)}
	for _, name := range sortedHeaderNames(headers) {
		parts = append(parts, "-H", shellQuote(name+": "+headers[name]))
	}
	if body != "" {
		parts = append(parts, "--data-raw", shellQuote(body))
	}
	return strings.Join(parts, " ")
}

func renderPython(method, target string, headers map[string]string, body string) (string, error) {
	return renderPythonStyle("import requests\n\nresponse = requests.request(\n", method, target, headers, body, "")
}

func renderCurlCFFI(method, target string, headers map[string]string, body string) (string, error) {
	return renderPythonStyle(
		"from curl_cffi import requests\n\nresponse = requests.request(\n",
		method,
		target,
		headers,
		body,
		",\n    impersonate=\"chrome\"",
	)
}

func renderPythonStyle(prefix, method, target string, headers map[string]string, body, extra string) (string, error) {
	headersJSON, err := json.MarshalIndent(headers, "    ", "    ")
	if err != nil {
		return "", fmt.Errorf("encode Python headers: %w", err)
	}
	methodJSON, _ := json.Marshal(method)
	targetJSON, _ := json.Marshal(target)
	var output strings.Builder
	output.WriteString(prefix)
	fmt.Fprintf(&output, "    method=%s,\n    url=%s,\n    headers=%s", methodJSON, targetJSON, headersJSON)
	if body != "" {
		bodyJSON, _ := json.Marshal(body)
		fmt.Fprintf(&output, ",\n    data=%s", bodyJSON)
	}
	if extra != "" {
		output.WriteString(extra)
	}
	output.WriteString(",\n)\n\nprint(response.status_code)\nprint(response.text)\n")
	return output.String(), nil
}

func renderJavaScript(method, target string, headers map[string]string, body string) (string, error) {
	options := struct {
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers,omitempty"`
		Body    string            `json:"body,omitempty"`
	}{
		Method:  method,
		Headers: headers,
		Body:    body,
	}
	optionsJSON, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode JavaScript options: %w", err)
	}
	targetJSON, _ := json.Marshal(target)
	return fmt.Sprintf(
		"const response = await fetch(%s, %s);\nconsole.log(response.status);\nconsole.log(await response.text());\n",
		targetJSON,
		optionsJSON,
	), nil
}

func renderAxios(method, target string, headers map[string]string, body string) (string, error) {
	options := struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers,omitempty"`
		Data    string            `json:"data,omitempty"`
	}{
		Method:  strings.ToLower(method),
		URL:     target,
		Headers: headers,
		Data:    body,
	}
	optionsJSON, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode axios options: %w", err)
	}
	return fmt.Sprintf(
		"import axios from \"axios\";\n\nconst response = await axios(%s);\nconsole.log(response.status);\nconsole.log(response.data);\n",
		optionsJSON,
	), nil
}

func renderHTTPie(method, target string, headers map[string]string, body string) string {
	parts := []string{"http", method, shellQuote(target)}
	for _, name := range sortedHeaderNames(headers) {
		parts = append(parts, shellQuote(name+":"+headers[name]))
	}
	if body != "" {
		parts = append(parts, "--raw", shellQuote(body))
	}
	return strings.Join(parts, " ")
}

func sortedHeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
