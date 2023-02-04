// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"testing"

	aws "github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

const (
	testSubId         = "test-subscription-id"
	testResourceGroup = "test-resource-group"
	testAccountId     = "test-account-id"
	testRegion        = "test-region"
)

func Test_ParseProviders(t *testing.T) {
	azureProvider := &azure.Provider{
		SubscriptionID: testSubId,
		ResourceGroup:  testResourceGroup,
	}
	awsProvider := &aws.Provider{
		AccountId:    testAccountId,
		TargetRegion: testRegion,
	}
	testCases := []struct {
		testname      string
		workspace     *workspaces.Workspace
		azureProvider *azure.Provider
		awsProvider   *aws.Provider
	}{
		{
			testname: "test-parse-azure-provider",
			workspace: &workspaces.Workspace{
				ProviderConfig: workspaces.ProviderConfig{},
			},
			azureProvider: azureProvider,
		},
		{
			testname: "test-parse-aws-provider",
			workspace: &workspaces.Workspace{
				ProviderConfig: workspaces.ProviderConfig{},
			},
			awsProvider: awsProvider,
		},
		{
			testname: "test-parse-multiple-providers",
			workspace: &workspaces.Workspace{
				ProviderConfig: workspaces.ProviderConfig{},
			},
			azureProvider: azureProvider,
			awsProvider:   awsProvider,
		},
	}
	for _, tt := range testCases {
		parseProviders(tt.workspace, []interface{}{tt.azureProvider, tt.awsProvider})
		if tt.azureProvider != nil {
			require.NotNil(t, tt.workspace.ProviderConfig.Azure)
			require.Equal(t, tt.workspace.ProviderConfig.Azure.SubscriptionID, azureProvider.SubscriptionID)
			require.Equal(t, tt.workspace.ProviderConfig.Azure.ResourceGroup, azureProvider.ResourceGroup)
		}
		if tt.awsProvider != nil {
			require.NotNil(t, tt.workspace.ProviderConfig.AWS)
			require.Equal(t, tt.workspace.ProviderConfig.AWS.AccountId, awsProvider.AccountId)
			require.Equal(t, tt.workspace.ProviderConfig.AWS.Region, awsProvider.TargetRegion)
		}
	}
}
