// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

type radiusOptions struct {
	test.TestOptions
	ARMAuthorizer       autorest.Authorizer
	Workspace           workspaces.Workspace
	AzureSubscriptionID string
	AzureResourceGroup  string
}

func NewK8sTestOptions(t *testing.T) radiusOptions {
	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")

	require.NotNil(t, workspace.ProviderConfig.Azure, "Azure config is nil")

	azureSubscriptionID := workspace.ProviderConfig.Azure.SubscriptionID
	require.NotEmpty(t, azureSubscriptionID, "subscription id must be specified")

	azureResourceGroup := workspace.ProviderConfig.Azure.ResourceGroup
	require.NotEmpty(t, azureResourceGroup, "resource group must be specified")

	return radiusOptions{
		TestOptions:         test.NewTestOptions(t),
		ARMAuthorizer:       auth,
		Workspace:           *workspace,
		AzureSubscriptionID: azureSubscriptionID,
		AzureResourceGroup:  azureResourceGroup,
	}
}

func Test_Deploy_AzureResources(t *testing.T) {
	applicationName := "test-app"
	template := "testdata/azure-resources-storage-account.bicep"
	params := fmt.Sprintf("storageAccountName=test%d", time.Now().Nanosecond())
	opt := NewK8sTestOptions(t)
	ctx := context.TODO()

	de := step.NewDeployExecutor(template, params)

	t.Run(de.GetDescription(), func(t *testing.T) {
		de.Execute(ctx, t, opt.TestOptions)

		validation.ValidateAzureResourcesCreated(ctx,
			t,
			opt.ARMAuthorizer,
			opt.AzureSubscriptionID,
			opt.AzureResourceGroup,
			applicationName,
			validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type:        azresources.StorageStorageAccounts,
						UserManaged: false,
					},
				},
			},
		)
	})
}
