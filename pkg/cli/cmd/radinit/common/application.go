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

package common

import (
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	ConfirmSetupApplicationPrompt   = "Setup application in the current directory?"
	EnterApplicationNamePrompt      = "Enter an application name"
	enterApplicationNamePlaceholder = "Enter application name..."
)

// EnterApplicationOptions prompts the user to scaffold an application and returns the scaffold flag and app name.
func EnterApplicationOptions(prompter prompt.Interface) (scaffold bool, name string, err error) {
	scaffold, err = prompt.YesOrNoPrompt(ConfirmSetupApplicationPrompt, prompt.ConfirmYes, prompter)
	if err != nil {
		return false, "", err
	}

	if !scaffold {
		return false, "", nil
	}

	chooseDefault := func() (string, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}

		return filepath.Base(wd), nil
	}

	name, err = EnterApplicationName(prompter, chooseDefault)
	if err != nil {
		return false, "", err
	}

	return scaffold, name, nil
}

// EnterApplicationName returns the application name based on the chooseDefault function. If the value returned by
// chooseDefault is not a valid application name, the user will be prompted.
func EnterApplicationName(prompter prompt.Interface, chooseDefault func() (string, error)) (string, error) {
	name, err := chooseDefault()
	if err != nil {
		return "", err
	}

	err = prompt.ValidateApplicationName(name)
	if err == nil {
		return name, nil
	}

	name, err = prompter.GetTextInput(EnterApplicationNamePrompt, prompt.TextInputOptions{
		Placeholder: enterApplicationNamePlaceholder,
		Validate:    prompt.ValidateApplicationName,
	})
	if err != nil {
		return "", err
	}

	return name, nil
}
