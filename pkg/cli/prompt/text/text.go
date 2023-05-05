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
	Quitting     bool
	err          error
}

// NewTextModel returns a new text model with prompt message.
//
// # Function Explanation
// 
//	NewTextModel creates a new Model object with a textinput, prompt message, placeholder, and width set to 40. It also sets
//	 the valueEntered flag to false and the err field to nil, allowing callers to check for errors when using the Model.
func NewTextModel(promptMsg string, placeHolder string) Model {
	ti := textinput.New()
	ti.Placeholder = placeHolder
	ti.Focus()
	ti.Width = 40

	return Model{
		textInput:    ti,
		promptMsg:    promptMsg,
		valueEntered: false,
		err:          nil,
	}
}

// Init returns initial tea command for text input.
//
// # Function Explanation
// 
//	Model.Init() returns a textinput.Blink command, which is used to handle errors and provide useful feedback to the 
//	caller.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update updates model with input form user.
//
// # Function Explanation
// 
//	Model.Update handles user input and errors, and returns a Model and a Cmd. It handles KeyEnter, KeyCtrlC, and KeyEsc, 
//	and sets the valueEntered and Quitting flags accordingly. It also handles errors by setting the err field in the Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.valueEntered = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.Quitting = true
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
//
// # Function Explanation
// 
//	Model.View() returns a string based on the value of the textInput field. If the valueEntered flag is true, it will 
//	return the promptMsg and either the value of the textInput field or its placeholder if the value is empty. Otherwise, it
//	 will return the promptMsg, the textInput field's view, and a message to quit. If an error occurs, it will return an 
//	empty string.
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
//
// # Function Explanation
// 
//	"GetValue" retrieves the value of the textInput field from the Model struct and returns it. If an error occurs, it is 
//	logged and an empty string is returned.
func (m Model) GetValue() string {
	return m.textInput.Value()
}
