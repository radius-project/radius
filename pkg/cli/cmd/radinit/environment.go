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
	"errors"
	"sort"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

const (
	selectExistingEnvironmentPrompt         = "Select an existing environment or create a new one"
	selectExistingEnvironmentCreateSentinel = "[create new]"
	enterNamespacePrompt                    = "Enter a namespace name to deploy apps into"
	enterEnvironmentNamePrompt              = "Enter an environment name"
	defaultEnvironmentName                  = "default"
	defaultEnvironmentNamespace             = "default"
)

func (r *Runner) enterEnvironmentOptions(ctx context.Context, workspace *workspaces.Workspace, options *initOptions) error {
	options.Environment.Create = true
	if !options.Cluster.Install {
		// If Radius is already installed then look for an existing environment first.
		name, err := r.selectExistingEnvironment(ctx, workspace)
		if err != nil {
			return err
		}

		// For an existing environment we won't make changes, so we're done gathering options.
		if name != nil {
			options.Environment.Name = *name
			options.Environment.Create = false
			return nil
		}
	}

	var err error
	options.Environment.Name, err = r.enterEnvironmentName(ctx)
	if err != nil {
		return err
	}

	options.Environment.Namespace, err = r.enterEnvironmentNamespace(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (r *Runner) selectExistingEnvironment(ctx context.Context, workspace *workspaces.Workspace) (*string, error) {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *workspace)
	if err != nil {
		return nil, err
	}

	environments, err := client.ListEnvironmentsAll(ctx)
	if err != nil {
		return nil, err
	}

	// If there are any existing environments ask to use one of those first.
	if len(environments) == 0 {
		return nil, nil
	}

	// Without any flags we take the default without asking if it's an option.
	//
	// The best way to accomplish that is to check if there's an environment named "default"
	// If not, we prompt the user for an input of remaining options
	if !r.Full {
		for _, env := range environments {
			if strings.EqualFold(defaultEnvironmentName, *env.Name) {
				return env.Name, nil
			}
		}
	}

	items := r.buildExistingEnvironmentList(environments)
	name, err := r.Prompter.GetListInput(items, selectExistingEnvironmentPrompt)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	if name == selectExistingEnvironmentCreateSentinel {
		// Returing nil tells the caller to create a new one.
		return nil, nil
	}

	return &name, nil
}

func (r *Runner) buildExistingEnvironmentList(existing []corerp.EnvironmentResource) []string {
	// Build the list of items in the following way:
	//
	// - default environment (if it exists)
	// - (all other existing environments)
	// - [create new]
	others := []string{}
	defaultExists := false
	for _, env := range existing {
		if strings.EqualFold(defaultEnvironmentName, *env.Name) {
			defaultExists = true
			continue
		}

		others = append(others, *env.Name)
	}
	sort.Strings(others)

	items := []string{}
	if defaultExists {
		items = append(items, defaultEnvironmentName)
	}
	items = append(items, others...)
	items = append(items, selectExistingEnvironmentCreateSentinel)

	return items
}

func (r *Runner) enterEnvironmentName(ctx context.Context) (string, error) {
	// When no flags are specified we don't ask for a name, just use 'default'
	if !r.Full {
		return defaultEnvironmentName, nil
	}

	name, err := r.Prompter.GetTextInput(enterEnvironmentNamePrompt, prompt.TextInputOptions{
		Default:     defaultEnvironmentName,
		Placeholder: defaultEnvironmentName,
		Validate:    prompt.ValidateResourceNameOrDefault,
	})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return "", &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return "", &cli.FriendlyError{Message: "Failed to read env name"}
	}

	return name, nil
}

func (r *Runner) enterEnvironmentNamespace(ctx context.Context) (string, error) {
	// When no flags are specified we don't want to ask about namespaces.
	if !r.Full {
		return defaultEnvironmentNamespace, nil
	}

	namespace, err := r.Prompter.GetTextInput(enterNamespacePrompt, prompt.TextInputOptions{
		Default:     defaultEnvironmentNamespace,
		Placeholder: defaultEnvironmentNamespace,
		Validate:    prompt.ValidateResourceNameOrDefault,
	})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return "", &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return "", &cli.FriendlyError{Message: "Namespace not specified"}
	}

	return namespace, nil
}
