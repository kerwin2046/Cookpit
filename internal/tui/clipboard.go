package tui

import "fmt"

// Clipboard copies text to the system clipboard.
type Clipboard interface {
	WriteAll(text string) error
}

type memoryClipboard struct {
	last string
}

func (c *memoryClipboard) WriteAll(text string) error {
	if text == "" {
		return fmt.Errorf("nothing to copy")
	}
	c.last = text
	return nil
}
