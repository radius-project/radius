// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package prompt

import (
	"errors"
	"fmt"
	"strings"
)

// EmptyValidator is a validation func that always returns true.
func EmptyValidator(string) (bool, error) {
	return true, nil
}

// Confirm prompts the user to confirm the answer to a yes/no question.
func Confirm(prompt string) (bool, error) {
	confirmed := false
	for {
		fmt.Print(prompt)
		fmt.Print(" ")

		input := ""
		count, err := fmt.Scanln(&input)
		if err != nil {
			return false, err
		} else if count == 0 {
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

// Text prompts the user to enter some freeform text.
func Text(prompt string, validator func(string) (bool, error)) (string, error) {
	return TextWithDefault(prompt, nil, validator)
}

// TextWithDefault prompts the user to enter some freeform text while offering a default value to set when the user doesn't enter any input (sends '\n')
func TextWithDefault(prompt string, defaultValue *string, validator func(string) (bool, error)) (string, error) {
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

		valid, err := validator(input)
		if err != nil {
			return "", err
		} else if valid {
			break
		}
	}
	return input, nil
}

// Select prompts the user to choose from the possible options
func Select(prompt string, choices []string) (int, error) {
	fmt.Println(prompt)
	fmt.Println("")

	for i, c := range choices {
		fmt.Printf("\t%3d: %v\n", i, c)
	}

	selected := 0
	for {
		fmt.Print("Enter a # to make your choice: ")

		count, err := fmt.Scanln(&selected)
		if err != nil {
			return 0, err
		} else if count == 0 {
			return 0, errors.New("nothing selected")
		}

		if selected >= 0 && selected < len(choices) {
			break
		}

		fmt.Printf("%d is not a valid choice\n", selected)
	}

	return selected, nil
}
