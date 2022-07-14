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
)

type BinaryAnswer int

const (
	unknown              = -1
	Yes     BinaryAnswer = iota
	No

	InvalidResourceNameMessage = "names must be made up of alphanumeric characters and hyphens, and must begin with an alphabetic character and end with an alphanumeric character"
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
			return false, errors.New("nothing enterted")
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
