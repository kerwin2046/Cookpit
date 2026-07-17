package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run code = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Bridge authenticated Chrome cookies") {
		t.Fatalf("help output = %q", stdout.String())
	}
}
