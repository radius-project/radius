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

	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_enterCloudProviderOptions(t *testing.T) {
	azureProviderServicePrincipal := azure.Provider{
		SubscriptionID: "test-subscription-id",
		ResourceGroup:  "test-resource-group",
		CredentialKind: azure.AzureCredentialKindServicePrincipal,
		ServicePrincipal: &azure.ServicePrincipalCredential{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			TenantID:     "test-tenant-id",
		},
	}

	azureProviderWorkloadIdentity := azure.Provider{
		SubscriptionID: "test-subscription-id",
		ResourceGroup:  "test-resource-group",
		CredentialKind: azure.AzureCredentialKindWorkloadIdentity,
		WorkloadIdentity: &azure.WorkloadIdentityCredential{
			ClientID: "test-client-id",
			TenantID: "test-tenant-id",
		},
	}

	awsProviderAccessKey := aws.Provider{
		Region:         "test-region",
		CredentialKind: "AccessKey",
		AccessKey: &aws.AccessKeyCredential{
			AccessKeyID:     "test-access-key-id",
			SecretAccessKey: "test-secret-access-key",
		},
		AccountID: "test-account-id",
	}

	awsProviderIRSA := aws.Provider{
		Region:         "test-region",
		CredentialKind: "IRSA",
		IRSA: &aws.IRSACredential{
			RoleARN: "test-role-arn",
		},
		AccountID: "test-account-id",
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

	t.Run("--full - aws provider - accesskey", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProviderAccessKey(prompter, awsClient, awsProviderAccessKey)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.Azure)
		require.Equal(t, awsProviderAccessKey, *options.CloudProviders.AWS)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("--full - aws provider - irsa", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProviderIRSA(prompter, awsClient, awsProviderIRSA)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.Azure)
		require.Equal(t, awsProviderIRSA, *options.CloudProviders.AWS)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("--full - azure provider - service principal", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, azure.ProviderDisplayName)
		setAzureCloudProviderServicePrincipal(prompter, azureClient, azureProviderServicePrincipal)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Equal(t, azureProviderServicePrincipal, *options.CloudProviders.Azure)

		expectedWrites := []any{
			output.LogOutput{
				Format: azureServicePrincipalCreateInstructionsFmt,
				Params: []any{azureProviderServicePrincipal.SubscriptionID, azureProviderServicePrincipal.ResourceGroup},
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("--full - azure provider - workload identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		awsClient := aws.NewMockClient(ctrl)
		azureClient := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, awsClient: awsClient, azureClient: azureClient, Output: &outputSink, Full: true}

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, azure.ProviderDisplayName)
		setAzureCloudProviderWorkloadIdentity(prompter, azureClient, azureProviderWorkloadIdentity)
		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Nil(t, options.CloudProviders.AWS)
		require.Equal(t, azureProviderWorkloadIdentity, *options.CloudProviders.Azure)

		expectedWrites := []any{
			output.LogOutput{
				Format: azureWorkloadIdentityCreateInstructionsFmt,
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
		setAWSCloudProviderAccessKey(prompter, awsClient, awsProviderAccessKey)

		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, azure.ProviderDisplayName)
		setAzureCloudProviderServicePrincipal(prompter, azureClient, azureProviderServicePrincipal)

		initAddCloudProviderPromptNo(prompter)

		options := initOptions{Environment: environmentOptions{Create: true}}
		err := runner.enterCloudProviderOptions(context.Background(), &options)
		require.NoError(t, err)
		require.Equal(t, awsProviderAccessKey, *options.CloudProviders.AWS)
		require.Equal(t, azureProviderServicePrincipal, *options.CloudProviders.Azure)

		expectedWrites := []any{
			output.LogOutput{
				Format: awsAccessKeysCreateInstructionFmt,
			},
			output.LogOutput{
				Format: azureServicePrincipalCreateInstructionsFmt,
				Params: []any{azureProviderServicePrincipal.SubscriptionID, azureProviderServicePrincipal.ResourceGroup},
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
		setAWSCloudProviderAccessKey(prompter, awsClient, awsProviderAccessKey)

		awsProvider := awsProviderAccessKey
		awsProvider.Region = "another-region"
		initAddCloudProviderPromptYes(prompter)
		initSelectCloudProvider(prompter, aws.ProviderDisplayName)
		setAWSCloudProviderAccessKey(prompter, awsClient, awsProvider)

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
