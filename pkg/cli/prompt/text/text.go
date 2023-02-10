// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package text

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type (
	errMsg error
)

var (
	QuitTextStyle = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

// Model is text model for bubble tea.
type Model struct {
	textInput    textinput.Model
	promptMsg    string
	valueEntered bool
	err          error
}

// NewTextModel returns a new text model with prompt message.
func NewTextModel(promptMsg string, placeHolder string) Model {
	ti := textinput.New()
	ti.Placeholder = placeHolder
	ti.Focus()
	ti.Width = 20

	return Model{
		textInput:    ti,
		promptMsg:    promptMsg,
		valueEntered: false,
		err:          nil,
	}
}

// Init returns initial tea command for text input.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update updates model with input form user.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.valueEntered = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

	// We handle errors just like any other message
	case errMsg:
		m.err = msg
		return m, nil
	}
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders a view with user selected value.
func (m Model) View() string {
	if m.valueEntered {
		if m.textInput.Value() == "" {
			return QuitTextStyle.Render(fmt.Sprintf("%s: %s", m.promptMsg, m.textInput.Placeholder))
		} else {
			return QuitTextStyle.Render(fmt.Sprintf("%s: %s", m.promptMsg, m.textInput.Value()))
		}

	}
	return fmt.Sprintf("%s\n\n%s\n\n%s", m.promptMsg, m.textInput.View(), "(esc to quit)")
}

// GetValue returns the input from the user.
func (m Model) GetValue() string {
	return m.textInput.Value()
}
