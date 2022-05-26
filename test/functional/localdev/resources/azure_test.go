// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

type localDevOptions struct {
	test.TestOptions
	ARMAuthorizer autorest.Authorizer
	Environment   *environments.LocalEnvironment
}

func NewLocalDevTestOptions(t *testing.T) localDevOptions {
	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	localEnv, ok := env.(*environments.LocalEnvironment)
	require.Truef(t, ok, "a standalone environment is required but the kind was '%v'", env.GetKind())

	return localDevOptions{
		TestOptions:   test.NewTestOptions(t),
		ARMAuthorizer: auth,
		Environment:   localEnv,
	}
}

func Test_Deploy_AzureResources(t *testing.T) {
	applicationName := "test-app"
	template := "testdata/azure-resources-storage-account.bicep"
	ctx := context.TODO()
	opt := NewLocalDevTestOptions(t)
	de := step.NewDeployExecutor(template, "storageAccountName=asilverman123445")
	t.Run(de.GetDescription(), func(t *testing.T) {
		de.Execute(ctx, t, opt.TestOptions)

		validation.ValidateAzureResourcesCreated(ctx,
			t,
			opt.ARMAuthorizer,
			opt.Environment.Providers.AzureProvider.SubscriptionID,
			opt.Environment.Providers.AzureProvider.ResourceGroup,
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
