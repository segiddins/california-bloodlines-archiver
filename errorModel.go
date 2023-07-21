package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrorModel is a model that represents an error.
type ErrorModel struct {
	error
}

// Init implements tea.Model.
func (ErrorModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m ErrorModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

// View implements tea.Model.
func (m ErrorModel) View() string {
	return errorStyle.Render(m.Error())
}

var _ tea.Model = ErrorModel{}

var errorStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FF0000")).
	PaddingTop(2)
