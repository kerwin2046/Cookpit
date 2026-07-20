//go:build linux

package tui

import (
	"fmt"

	"github.com/atotto/clipboard"
)

type systemClipboard struct{}

func defaultClipboard() Clipboard {
	return systemClipboard{}
}

func (systemClipboard) WriteAll(text string) error {
	if text == "" {
		return fmt.Errorf("nothing to copy")
	}
	return clipboard.WriteAll(text)
}
