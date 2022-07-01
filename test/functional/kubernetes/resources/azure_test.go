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
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

type radiusOptions struct {
	test.TestOptions
	ARMAuthorizer autorest.Authorizer
	Environment   environments.Environment
}

func NewK8sTestOptions(t *testing.T) radiusOptions {
	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	return radiusOptions{
		TestOptions:   test.NewTestOptions(t),
		ARMAuthorizer: auth,
		Environment:   env,
	}
}

func Test_Deploy_AzureResources(t *testing.T) {
	applicationName := "test-app"
	template := "testdata/azure-resources-storage-account.bicep"
	params := fmt.Sprintf("storageAccountName=test%d", time.Now().Nanosecond())
	opt := NewK8sTestOptions(t)
	ctx := context.TODO()

	providers := opt.Environment.GetProviders()
	de := step.NewDeployExecutor(template, params)

	t.Run(de.GetDescription(), func(t *testing.T) {
		de.Execute(ctx, t, opt.TestOptions)

		validation.ValidateAzureResourcesCreated(ctx,
			t,
			opt.ARMAuthorizer,
			providers.AzureProvider.SubscriptionID,
			providers.AzureProvider.ResourceGroup,
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
