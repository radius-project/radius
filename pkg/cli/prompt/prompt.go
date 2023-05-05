// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package prompt

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	cli_list "github.com/project-radius/radius/pkg/cli/prompt/list"
	"github.com/project-radius/radius/pkg/cli/prompt/text"
)

type BinaryAnswer int

const (
	unknown              = -1
	Yes     BinaryAnswer = iota
	No

	InvalidResourceNameMessage = "name must be made up of alphanumeric characters and hyphens, and must begin with an alphabetic character and end with an alphanumeric character"
	ErrExitConsoleMessage      = "exiting command"
)

// # Function Explanation
// 
//	MatchAll takes in a variable number of validator functions and returns a single validator function that checks if all of
//	 the validators return true. If any of the validators return false, the returned validator will also return false. If 
//	any of the validators return an error, the returned validator will also return an error.
func MatchAll(validators ...func(string) (bool, string, error)) func(string) (bool, string, error) {
	return func(input string) (bool, string, error) {
		for _, validator := range validators {
			result, message, err := validator(input)
			if err != nil {
				return false, "", err
			} else if !result {
				return false, message, nil
			}
		}

		return true, "", nil
	}
}

// EmptyValidator is a validation func that always returns true.
//
// # Function Explanation
// 
//	"EmptyValidator" is a function that always returns true, an empty string, and no error, making it useful for callers who
//	 need to check if a string is valid but don't need to know the details of the validation. If an error occurs, it will be
//	 returned to the caller for further handling.
func EmptyValidator(string) (bool, string, error) {
	return true, "", nil
}

// Largely matches https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules
//
// # Function Explanation
// 
//	ResourceName checks if the given input string matches a regular expression pattern and returns a boolean value 
//	indicating the result, an error message, and an error. If the input does not match the pattern, the boolean value will 
//	be false and the error message will be "InvalidResourceNameMessage".
func ResourceName(input string) (bool, string, error) {
	r := regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9-]*[a-zA-Z0-9]$")
	return r.MatchString(input), InvalidResourceNameMessage, nil
}

// # Function Explanation
// 
//	UUIDv4Validator checks if a given string is a valid UUIDv4 and returns a boolean, an error message and an error. If the 
//	string is not a valid UUIDv4, the boolean will be false and the error message will be "input is not a valid uuid". If 
//	the string is valid, the boolean will be true and the error will be nil.
func UUIDv4Validator(uuid string) (bool, string, error) {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	return r.MatchString(uuid), "input is not a valid uuid", nil
}

// Confirm prompts the user to confirm the answer to a yes/no question.
//
// # Function Explanation
// 
//	ConfirmWithDefault prompts the user for a yes/no answer and returns a boolean based on the user's input. If the user 
//	does not provide an answer, the default answer is used. If an error occurs, an error is returned.
func ConfirmWithDefault(prompt string, defaultAns BinaryAnswer) (bool, error) {
	confirmed := false
	for {
		fmt.Print(prompt)
		fmt.Print(" ")

		input := ""
		count, err := fmt.Scanln(&input)
		if count == 0 && defaultAns != unknown {
			return defaultAns == Yes, nil
		} else if err != nil {
			return false, errors.New("nothing entered")
		}

		if strings.EqualFold("y", input) {
			confirmed = true
			break
		} else if strings.EqualFold("n", input) {
			confirmed = false
			break
		}
	}

	return confirmed, nil
}

// Confirm prompts the user to confirm the answer to a yes/no question.
//
// # Function Explanation
// 
//	Confirm prompts the user with a given string and returns a boolean value based on the user's response, with an error if 
//	the response is invalid.
func Confirm(prompt string) (bool, error) {
	return ConfirmWithDefault(prompt, unknown)
}

// Text prompts the user to enter some freeform text.
//
// # Function Explanation
// 
//	Text() prompts the user for input and validates it using the validator function provided, returning the input or an 
//	error if validation fails.
func Text(prompt string, validator func(string) (bool, string, error)) (string, error) {
	return TextWithDefault(prompt, nil, validator)
}

// TextWithDefault prompts the user to enter some freeform text while offering a default value to set when the user doesn't enter any input (sends '\n')
//
// # Function Explanation
// 
//	TextWithDefault prompts the user for input and validates it using the validator function. If the input is invalid, it 
//	prints an error message and prompts the user again. If no input is given and a default value is provided, it returns the
//	 default value. If an error occurs, it returns an error.
func TextWithDefault(prompt string, defaultValue *string, validator func(string) (bool, string, error)) (string, error) {
	input := ""
	for {
		fmt.Print(prompt)
		fmt.Print(" ")

		count, err := fmt.Scanln(&input)
		if count == 0 && defaultValue != nil {
			return *defaultValue, nil
		} else if err != nil {
			return "", errors.New("nothing entered")
		}

		valid, message, err := validator(input)
		if err != nil {
			return "", err
		} else if valid {
			break
		}

		if message != "" {
			fmt.Println(message)
		}
	}
	return input, nil
}

// Select prompts the user to choose from the possible options while offering a default value when the user doesn't enter any input (sends '\n')
//
// # Function Explanation
// 
//	SelectWithDefault prints a prompt and a list of choices, allowing the user to select one of the choices or use a default
//	 choice if provided. It returns the index of the selected choice or an error if nothing is selected.
func SelectWithDefault(prompt string, defaultChoice *string, choices []string) (int, error) {
	fmt.Println(prompt)
	fmt.Println("")
	var defaultSelection int
	for i, c := range choices {
		if defaultChoice != nil && c == *defaultChoice {
			defaultSelection = i
		}
		fmt.Printf("\t%3d: %v\n", i, c)
	}

	selected := 0
	for {
		fmt.Printf("Enter a # to make your choice [%d]: ", defaultSelection)

		count, err := fmt.Scanln(&selected)
		if count == 0 && defaultChoice != nil {
			return defaultSelection, nil
		} else if err != nil {
			return 0, errors.New("nothing selected")
		}

		if selected >= 0 && selected < len(choices) {
			break
		}

		fmt.Printf("%d is not a valid choice\n", selected)
	}

	return selected, nil
}

//go:generate mockgen -destination=./mock_prompter.go -package=prompt -self_package github.com/project-radius/radius/pkg/cli/prompt github.com/project-radius/radius/pkg/cli/prompt Interface

// Interface contains operation to get user inputs for cli
type Interface interface {
	// GetTextInput prompts user for a text input
	GetTextInput(promptMsg string, defaultPlaceHolder string) (string, error)

	// GetListInput prompts user to select from a list
	GetListInput(items []string, promptMsg string) (string, error)
}

// Impl implements BubbleTeaPrompter
type Impl struct{}

// GetTextInput prompts user for a text input
//
// # Function Explanation
// 
//	GetTextInput prompts the user for input with a given prompt message and default placeholder, and returns the user's 
//	input as a string. If an error occurs, it is returned to the caller, including an error if the user exits the console.
func (i *Impl) GetTextInput(promptMsg string, defaultPlaceHolder string) (string, error) {
	// TODO: implement text model
	tm := text.NewTextModel(promptMsg, defaultPlaceHolder)

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

var _ error = (*ErrExitConsole)(nil)

// ErrExitConsole represents interrupt commands being entered.
type ErrExitConsole struct {
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
// # Function Explanation
// 
//	YesOrNoPrompt prompts the user to select from a list of two options, "Yes" or "No", with the default option being the 
//	one specified in the parameters. If an error occurs, it is returned to the caller.
func YesOrNoPrompt(promptMsg string, defaultString string, prompter Interface) (bool, error) {
	var valueList []string
	if strings.EqualFold("Yes", defaultString) {
		valueList = []string{"Yes", "No"}
	} else {
		valueList = []string{"No", "Yes"}
	}
	input, err := prompter.GetListInput(valueList, promptMsg)
	if err != nil {
		return false, err
	}
	return strings.EqualFold("Yes", input), nil
}
