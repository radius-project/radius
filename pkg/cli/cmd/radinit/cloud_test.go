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
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
)

func Test_enterCloudProviderOptions(t *testing.T) {
	azureProvider := azure.Provider{
		SubscriptionID: "test-subscription-id",
		ResourceGroup:  "test-resource-group",
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			TenantID:     "test-tenant-id",
		},
	}

	awsProvider := aws.Provider{
		Region:          "test-region",
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
		AccountID:       "test-account-id",
	}

	t.Run("cloud providers skipped when no flags specified", func(t *testing.T) {
		runner := Runner{}

		options := initOptions{}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Nil(t, options.CloudProviders.Azure)
	})

	t.Run("--full - cloud providers skipped for existing environment", func(t *testing.T) {
		runner := Runner{Full: true}

		options := initOptions{Environment: environmentOptions{Create: false}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Nil(t, options.CloudProviders.Azure)
	})

	t.Run("--full - no providers added", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Nil(t, options.CloudProviders.Azure)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("--full - no providers added (back)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, confirmCloudProviderBackNavigationSentinel)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Nil(t, options.CloudProviders.Azure)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("--full - aws provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProvider(prompter, awsClient, awsProvider)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.Azure)
		require.Equal(t, awsProvider, *options.CloudProviders.AWS)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("--full - azure provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, azure.ProviderDisplayName)
		setAzureCloudProvider(prompter, azureClient, azureProvider)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Equal(t, azureProvider, *options.CloudProviders.Azure)

		expectedWrites := []any{
			output.LogOutput{
				Format: azureServicePrincipalCreateInstructionsFmt,
				Params: []any{azureProvider.SubscriptionID, azureProvider.ResourceGroup},
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("--full - both providers", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProvider(prompter, awsClient, awsProvider)

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, azure.ProviderDisplayName)
		setAzureCloudProvider(prompter, azureClient, azureProvider)

		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Equal(t, awsProvider, *options.CloudProviders.AWS)
		require.Equal(t, azureProvider, *options.CloudProviders.Azure)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
			output.LogOutput{
				Format: azureServicePrincipalCreateInstructionsFmt,
				Params: []any{azureProvider.SubscriptionID, azureProvider.ResourceGroup},
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	// Users can overwrite a previous choice by making the same selection.
	t.Run("--full - overwrite-provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProvider(prompter, awsClient, awsProvider)

		awsProvider := awsProvider
		awsProvider.Region = "another-region"
		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProvider(prompter, awsClient, awsProvider)

		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.Azure)
		require.Equal(t, awsProvider, *options.CloudProviders.AWS)
		require.Equal(t, "another-region", options.CloudProviders.AWS.Region)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})
}
