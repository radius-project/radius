/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package text

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	QuitTextStyle = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

// TextModelOptions contains options for the text model.
type TextModelOptions struct {
	// Default sets a default value for the user input.
	Default string

	// Placeholder sets a placeholder for the user input.
	Placeholder string

	// Validate defines a validator for the user input.
	Validate func(string) error

	// EchoMode indicates the input behavior of the text input field (e.g. password).
	EchoMode textinput.EchoMode
}

// Model is text model for bubble tea.
type Model struct {
	// Style configures the style applied to all rendering for the prompt. This can be used to apply padding and borders.
	Style lipgloss.Style

	// ErrStyle configures the style applied to error messages.
	ErrStyle lipgloss.Style

	// Quitting indicates whether the prompt has been canceled.
	Quitting bool

	options      TextModelOptions
	prompt       string
	textInput    textinput.Model
	valueEntered bool
}

// NewTextModel returns a new text model with prompt message.
func NewTextModel(prompt string, options TextModelOptions) Model {
	// Note: we don't use the validation support provided by textinput due to a bug in the library.
	//
	// See: https://github.com/charmbracelet/bubbles/issues/244
	//
	// This blocks the input control from **ever** containing invalid data. For example `prod-` is an invalid environment name,
	// so it will be blocked. This means you can't type `prod-aws` which is a valid name.
	ti := textinput.New()
	ti.Focus()
	ti.Width = 40
	ti.Placeholder = options.Placeholder
	ti.EchoMode = options.EchoMode

	return Model{
		Style:     lipgloss.NewStyle(), // No border or padding by default
		ErrStyle:  lipgloss.NewStyle().Width(80).Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}),
		options:   options,
		prompt:    prompt,
		textInput: ti,
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
			// Block submit on invalid values.
			if m.textInput.Err != nil {
				return m, nil
			}

			m.valueEntered = true
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.Quitting = true
			return m, tea.Quit
		}
	}

	// Workaround for https://github.com/charmbracelet/bubbles/issues/244
	//
	// Instead of using the validation support on textinput, we perform the validation
	// and update its state manually.
	m.textInput, cmd = m.textInput.Update(msg)
	if m.options.Validate != nil {
		validationErr := m.options.Validate(m.textInput.Value())
		m.textInput.Err = validationErr
	}

	return m, cmd
}

// View renders a view with user selected value.
func (m Model) View() string {
	if m.valueEntered {
		// Hide all of the input when complete.
		return ""
	}

	// Renders output like:
	//
	// Enter some data [prompt]:
	//
	// > [placeholder or input]
	//
	// (ctrl+c to quit)

	view := &strings.Builder{}
	view.WriteString(m.prompt)
	view.WriteString("\n\n")
	view.WriteString(m.textInput.View())
	view.WriteString("\n\n")
	view.WriteString("(ctrl+c to quit)")
	if m.textInput.Err != nil {
		view.WriteString("\n\n")
		view.WriteString("Error: ")
		view.WriteString(m.ErrStyle.Render(m.textInput.Err.Error()))
	}

	return m.Style.Render(view.String())
}

// GetValue returns the input from the user, or the default value if the user did not enter anything.
func (m Model) GetValue() string {
	value := m.textInput.Value()
	if value == "" {
		return m.options.Default
	}

	return value
}
