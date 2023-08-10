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

	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	confirmCloudProviderBackNavigationSentinel = "[back]"
	confirmCloudProviderPrompt                 = "Add cloud providers for cloud resources?"
	confirmCloudProviderAdditionalPrompt       = "Add additional cloud providers for cloud resources?"
	selectCloudProviderPrompt                  = "Select your cloud provider"
)

func (r *Runner) enterCloudProviderOptions(ctx context.Context, options *initOptions) error {
	// When no flags are specified we don't want to ask about cloud providers.
	if !r.Full {
		return nil
	}

	// If we're creating an environment we can't change cloud providers.
	if !options.Environment.Create {
		return nil
	}

	addingCloudProvider, err := prompt.YesOrNoPrompt(confirmCloudProviderPrompt, prompt.ConfirmNo, r.Prompter)
	if err != nil {
		return err
	}

	for addingCloudProvider {
		choices := []string{azure.ProviderDisplayName, aws.ProviderDisplayName, confirmCloudProviderBackNavigationSentinel}
		cloudProvider, err := r.Prompter.GetListInput(choices, selectCloudProviderPrompt)
		if err != nil {
			return err
		}

		switch cloudProvider {
		case azure.ProviderDisplayName:
			provider, err := r.enterAzureCloudProvider(ctx, options)
			if err != nil {
				return err
			}

			options.CloudProviders.Azure = provider
		case aws.ProviderDisplayName:
			provider, err := r.enterAWSCloudProvider(ctx, options)
			if err != nil {
				return err
			}

			options.CloudProviders.AWS = provider
		case confirmCloudProviderBackNavigationSentinel:
			return nil
		default:
			return errors.New("unsupported Cloud Provider")
		}

		addingCloudProvider, err = prompt.YesOrNoPrompt(confirmCloudProviderAdditionalPrompt, prompt.ConfirmNo, r.Prompter)
		if err != nil {
			return err
		}
	}

	return nil
}
