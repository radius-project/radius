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
	"github.com/manifoldco/promptui"
	cli_list "github.com/project-radius/radius/pkg/cli/prompt/list"
	"github.com/project-radius/radius/pkg/cli/prompt/text"
)

type BinaryAnswer int

const (
	unknown              = -1
	Yes     BinaryAnswer = iota
	No

	InvalidResourceNameMessage = "name must be made up of alphanumeric characters and hyphens, and must begin with an alphabetic character and end with an alphanumeric character"
)

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
func EmptyValidator(string) (bool, string, error) {
	return true, "", nil
}

// Largely matches https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules
func ResourceName(input string) (bool, string, error) {
	r := regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9-]*[a-zA-Z0-9]$")
	return r.MatchString(input), InvalidResourceNameMessage, nil
}

func UUIDv4Validator(uuid string) (bool, string, error) {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	return r.MatchString(uuid), "input is not a valid uuid", nil
}

// Confirm prompts the user to confirm the answer to a yes/no question.
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
func Confirm(prompt string) (bool, error) {
	return ConfirmWithDefault(prompt, unknown)
}

// Text prompts the user to enter some freeform text.
func Text(prompt string, validator func(string) (bool, string, error)) (string, error) {
	return TextWithDefault(prompt, nil, validator)
}

// TextWithDefault prompts the user to enter some freeform text while offering a default value to set when the user doesn't enter any input (sends '\n')
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

//go:generate mockgen -destination=./mock_prompt.go -package=prompt -self_package github.com/project-radius/radius/pkg/cli/prompt github.com/project-radius/radius/pkg/cli/prompt Interface
type Interface interface {
	RunPrompt(prompter promptui.Prompt) (string, error)
	RunSelect(selector promptui.Select) (int, string, error)

	// Confirm prompts the user to confirm the answer to a yes/no question.
	ConfirmWithDefault(prompt string, defaultAns BinaryAnswer) (bool, error)
}

type Impl struct{}

// Prompts user for an input
func (i *Impl) RunPrompt(prompter promptui.Prompt) (string, error) {
	return prompter.Run()
}

// Prompts user to select from a list of values
func (i *Impl) RunSelect(selector promptui.Select) (int, string, error) {
	return selector.Run()
}

func (i *Impl) ConfirmWithDefault(prompt string, defaultAns BinaryAnswer) (bool, error) {
	return ConfirmWithDefault(prompt, defaultAns)
}

// Creates a Yes or No prompts where user has to enter either a Y or N as input
func YesOrNoPrompter(label string, defaultValue string, prompter Interface) (string, error) {
	textPrompt := promptui.Prompt{
		Label:    label,
		Default:  defaultValue,
		Validate: validateYesOrNo,
	}
	result, err := prompter.RunPrompt(textPrompt)
	if errors.Is(err, promptui.ErrAbort) {
		return result, nil
	} else if err != nil {
		return "", err
	}
	return result, err
}

func validateYesOrNo(s string) error {
	if s == "" || (strings.ToLower(s) != "y" && strings.ToLower(s) != "n") {
		return errors.New("invalid input")
	}
	return nil
}

// Creates a prompt where a user is asked for a value suggesting a default Val
func TextPromptWithDefault(label string, defaultVal string, f func(s string) (bool, string, error)) promptui.Prompt {
	return promptui.Prompt{
		Label:   label,
		Default: defaultVal,
		Validate: func(s string) error {
			valid, msg, err := f(s)
			if err != nil {
				return err
			}
			if !valid {
				return errors.New(msg)
			}
			return nil
		},
		AllowEdit: true,
	}
}

// Creates a prompter which has a list of values to select from
func SelectionPrompter(label string, items []string) promptui.Select {
	return promptui.Select{
		Label: label,
		Items: items,
		Searcher: func(input string, index int) bool {
			return strings.HasPrefix(items[index], input)
		},
	}
}

//go:generate mockgen -destination=./mock_inputprompter.go -package=prompt -self_package github.com/project-radius/radius/pkg/cli/prompt github.com/project-radius/radius/pkg/cli/prompt InputPrompter

// InputPrompter contains operation to get user inputs for cli
type InputPrompter interface {
	// GetTextInput prompts user for a text input
	GetTextInput(promptMsg string, defaultPlaceHolder string) (string, error)

	// GetListInput prompts user to select from a list
	GetListInput(items []string, promptMsg string) (string, error)
}

// InputPrompterImpl implements BubbleTeaPrompter
type InputPrompterImpl struct{}

// GetTextInput prompts user for a text input
func (i *InputPrompterImpl) GetTextInput(promptMsg string, defaultPlaceHolder string) (string, error) {
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
	fmt.Println("Entered value:", tm.GetValue())

	return tm.GetValue(), nil
}

// GetListInput prompts user to select from a list
func (i *InputPrompterImpl) GetListInput(items []string, promptMsg string) (string, error) {
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

	return lm.Choice, nil
}

// YesOrNoPrompt Creates a Yes or No prompt where user has to select either a Yes or No as input
// defaultString decides the first(default) value on the list.
func YesOrNoPrompt(promptMsg string, defaultString string, prompter InputPrompter) (bool, error) {
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
