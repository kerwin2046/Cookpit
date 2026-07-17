package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(opts Options) error {
	model, err := New(opts)
	if err != nil {
		return err
	}
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run playground UI: %w", err)
	}
	return nil
}
