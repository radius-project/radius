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

package prompt

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	cli_list "github.com/project-radius/radius/pkg/cli/prompt/list"
	"github.com/project-radius/radius/pkg/cli/prompt/text"
)

const (
	// ConfirmYes can be used with YesOrNoPrompt to create a confirmation dialog.
	ConfirmYes = "Yes"

	// ConfirmNo can be used with YesOrNoPrompt to create a confirmation dialog.
	ConfirmNo = "No"
)

//go:generate mockgen -destination=./mock_prompter.go -package=prompt -self_package github.com/project-radius/radius/pkg/cli/prompt github.com/project-radius/radius/pkg/cli/prompt Interface

// Interface contains operation to prompt the user interactively.
//
// All functions on Interface returns an *ErrExitConsole if the user cancels. This is a friendly error and does not need
// special handling by calling code.
type Interface interface {
	// GetTextInput prompts user for a text input. Will return ErrExitConsole if the user cancels.
	GetTextInput(prompt string, options TextInputOptions) (string, error)

	// GetListInput prompts user to select from a list. Will return ErrExitConsole if the user cancels.
	GetListInput(items []string, promptMsg string) (string, error)

	// RunProgram runs a bubbletea program and blocks until the program exits.
	//
	// To create a cancellable program, use the options to pass a context.Context into the program.
	RunProgram(program *tea.Program) (tea.Model, error)
}

// TextInputOptions contains options for 'Interface.GetTextInput'.
type TextInputOptions = text.TextModelOptions

// Impl implements BubbleTeaPrompter
type Impl struct{}

// GetTextInput prompts user for a text input
func (i *Impl) GetTextInput(prompt string, options TextInputOptions) (string, error) {
	tm := text.NewTextModel(prompt, options)

	// Give us some padding so we don't butt up against the user's command.
	tm.Style = lipgloss.NewStyle().PaddingTop(1)

	model, err := tea.NewProgram(tm).Run()
	if err != nil {
		return "", err
	}
	tm, ok := model.(text.Model)
	if !ok {
		return "", &ErrUnsupportedModel{}
	}
	if tm.Quitting {
		return "", &ErrExitConsole{}
	}

	return tm.GetValue(), nil
}

// GetListInput prompts user to select from a list
//
// # Function Explanation
// 
//	Impl.GetListInput creates a new ListModel with the given items and prompt message, then runs it using tea. If an error 
//	occurs, it is returned. Otherwise, the ListModel is checked for validity and if it is quitting, an error is returned. 
//	Otherwise, the choice from the ListModel is returned. If any errors occur, they should be handled by the caller.
func (i *Impl) GetListInput(items []string, promptMsg string) (string, error) {
	lm := cli_list.NewListModel(items, promptMsg)

	// Give us some padding so we don't butt up against the user's command.
	lm.Style = lipgloss.NewStyle().PaddingTop(1)

	lm.List.Styles = list.Styles{}
	model, err := tea.NewProgram(lm).Run()
	if err != nil {
		return "", err
	}

	lm, ok := model.(cli_list.ListModel)
	if !ok {
		return "", &ErrUnsupportedModel{}
	}
	if lm.Quitting {
		return "", &ErrExitConsole{}
	}

	return lm.Choice, nil
}

// RunProgram runs a bubbletea program and blocks until the program exits.
func (i *Impl) RunProgram(program *tea.Program) (tea.Model, error) {
	return program.Run()
}

var _ clierrors.FriendlyError = (*ErrExitConsole)(nil)

// ErrExitConsole represents interrupt commands being entered.
type ErrExitConsole struct {
}

// IsFriendlyError returns true. Cancelling a command prompt should always be handled gracefully by the CLI.
func (*ErrExitConsole) IsFriendlyError() bool {
	return true
}

// Error returns the error message.
//
// # Function Explanation
// 
//	ErrExitConsole is an error type that is returned when the function encounters an error that requires the program to exit
//	 the console. It provides a useful message to the callers of the function to inform them of the error.
func (e *ErrExitConsole) Error() string {
	return ErrExitConsoleMessage
}

// Is checks for the error type is ErrExitConsole.
//
// # Function Explanation
// 
//	ErrExitConsole is a custom error type that implements the error interface, allowing it to be used in error handling. It 
//	provides a way for callers of the function to check if the error is of this type and handle it accordingly.
func (e *ErrExitConsole) Is(target error) bool {
	_, ok := target.(*ErrExitConsole)
	return ok
}

// YesOrNoPrompt Creates a Yes or No prompt where user has to select either a Yes or No as input
// defaultString decides the first(default) value on the list.
//
// Returns an *ErrExitConsole if the user cancels. This is a friendly error and does not need
// special handling by calling code.
func YesOrNoPrompt(promptMsg string, defaultString string, prompter Interface) (bool, error) {
	var valueList []string
	if strings.EqualFold(ConfirmYes, defaultString) {
		valueList = []string{ConfirmYes, ConfirmNo}
	} else {
		valueList = []string{ConfirmNo, ConfirmYes}
	}
	input, err := prompter.GetListInput(valueList, promptMsg)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(ConfirmYes, input), nil
}
