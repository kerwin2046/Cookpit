//go:build !linux

package tui

import "fmt"

type systemClipboard struct{}

func defaultClipboard() Clipboard {
	return systemClipboard{}
}

func (systemClipboard) WriteAll(text string) error {
	return fmt.Errorf("clipboard is only supported on Linux in this build")
}
