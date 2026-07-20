package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	exporter "cookiex/internal/export"
	requestmodel "cookiex/internal/request"
)

type responseView int

const (
	respViewHeaders responseView = iota
	respViewBody
)

func FormatResponseHeaders(resp *requestmodel.Response) string {
	if resp == nil {
		return "Send a request to see the response."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n", resp.Status, resp.Duration.Round(time.Millisecond))
	names := make([]string, 0, len(resp.Headers))
	for name := range resp.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range resp.Headers.Values(name) {
			fmt.Fprintf(&b, "%s: %s\n", name, value)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatResponseBody(body []byte, truncated bool) string {
	if len(body) == 0 && !truncated {
		return "(empty body)"
	}
	formatted := body
	if json.Valid(body) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err == nil {
			formatted = buf.Bytes()
		}
	}
	out := string(formatted)
	if truncated {
		out += "\n[response body truncated]"
	}
	return out
}

func CopyTargetLabel(tab resultTab, view responseView, codeFormat int) string {
	switch tab {
	case tabRequest:
		return "request"
	case tabCode:
		return exporter.FormatLabel(exporter.SupportedFormats[codeFormat])
	default:
		if view == respViewHeaders {
			return "response headers"
		}
		return "response body"
	}
}
