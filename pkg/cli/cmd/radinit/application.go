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

package radinit

import (
	"context"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	confirmSetupApplicationPrompt   = "Setup application in the current directory?"
	enterApplicationNamePrompt      = "Enter an application name"
	enterApplicationNamePlaceholder = "Enter application name..."
)

func (r *Runner) enterApplicationOptions(ctx context.Context, options *initOptions) error {
	var err error
	options.Application.Scaffold, err = prompt.YesOrNoPrompt(confirmSetupApplicationPrompt, prompt.ConfirmYes, r.Prompter)
	if err != nil {
		return err
	}

	if !options.Application.Scaffold {
		return nil
	}

	chooseDefault := func() (string, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}

		return filepath.Base(wd), nil
	}

	options.Application.Name, err = r.enterApplicationName(chooseDefault)
	if err != nil {
		return err
	}

	return nil
}

// enterApplicationName returns the application name based on the chooseDefault function. If the value returned by
// chooseDefault is not a valid application name, the user will be prompted. chooseDefault is provided for testing
// purposes.
func (r *Runner) enterApplicationName(chooseDefault func() (string, error)) (string, error) {
	// We might have to prompt for an application name if the current directory is not a valid application name.
	// These cases should be rare but just in case...
	name, err := chooseDefault()
	if err != nil {
		return "", err
	}

	err = prompt.ValidateResourceName(name)
	if err == nil {
		// Default name is a valid application name.
		return name, nil
	}

	name, err = r.Prompter.GetTextInput(enterApplicationNamePrompt, prompt.TextInputOptions{
		Placeholder: enterApplicationNamePlaceholder,
		Validate:    prompt.ValidateResourceName,
	})
	if err != nil {
		return "", err
	}

	return name, nil
}
