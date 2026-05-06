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
	"errors"

	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	ConfirmCloudProviderBackNavigationSentinel = "[back]"
	ConfirmCloudProviderPrompt                 = "Add cloud providers for cloud resources?"
	ConfirmCloudProviderAdditionalPrompt       = "Add additional cloud providers for cloud resources?"
	SelectCloudProviderPrompt                  = "Select your cloud provider"
)

// CloudProviderResult holds the results of gathering cloud provider options.
type CloudProviderResult struct {
	Azure *azure.Provider
	AWS   *aws.Provider
}

// AzureProviderFunc is a callback to gather Azure provider config.
type AzureProviderFunc func() (*azure.Provider, error)

// AWSProviderFunc is a callback to gather AWS provider config.
type AWSProviderFunc func() (*aws.Provider, error)

// EnterCloudProviderOptions prompts the user to add cloud providers.
// If full is false or environmentCreate is false, it returns immediately with no providers.
func EnterCloudProviderOptions(prompter prompt.Interface, full bool, environmentCreate bool, enterAzure AzureProviderFunc, enterAWS AWSProviderFunc) (CloudProviderResult, error) {
	result := CloudProviderResult{}

	if !full {
		return result, nil
	}

	if !environmentCreate {
		return result, nil
	}

	addingCloudProvider, err := prompt.YesOrNoPrompt(ConfirmCloudProviderPrompt, prompt.ConfirmNo, prompter)
	if err != nil {
		return result, err
	}

	for addingCloudProvider {
		choices := []string{azure.ProviderDisplayName, aws.ProviderDisplayName, ConfirmCloudProviderBackNavigationSentinel}
		cloudProvider, err := prompter.GetListInput(choices, SelectCloudProviderPrompt)
		if err != nil {
			return result, err
		}

		switch cloudProvider {
		case azure.ProviderDisplayName:
			provider, err := enterAzure()
			if err != nil {
				return result, err
			}
			result.Azure = provider
		case aws.ProviderDisplayName:
			provider, err := enterAWS()
			if err != nil {
				return result, err
			}
			result.AWS = provider
		case ConfirmCloudProviderBackNavigationSentinel:
			return result, nil
		default:
			return result, errors.New("unsupported Cloud Provider")
		}

		addingCloudProvider, err = prompt.YesOrNoPrompt(ConfirmCloudProviderAdditionalPrompt, prompt.ConfirmNo, prompter)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}
