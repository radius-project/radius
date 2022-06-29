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
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

type k8sOptions struct {
	test.TestOptions
	ARMAuthorizer autorest.Authorizer
	Environment   *environments.RadiusEnvironment
}

func NewK8sTestOptions(t *testing.T) k8sOptions {
	auth, err := armauth.GetArmAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	k8sEnv, ok := env.(*environments.RadiusEnvironment)
	require.Truef(t, ok, "a standalone environment is required but the kind was '%v'", env.GetKind())

	return k8sOptions{
		TestOptions:   test.NewTestOptions(t),
		ARMAuthorizer: auth,
		Environment:   k8sEnv,
	}
}

func Test_Deploy_AzureResources(t *testing.T) {
	applicationName := "test-app"
	template := "testdata/azure-resources-storage-account.bicep"
	params := fmt.Sprintf("storageAccountName=test%d", time.Now().Nanosecond())
	opt := NewK8sTestOptions(t)

	test := kubernetes.NewApplicationTest(t, applicationName, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template, params),
			PostStepVerify: func(ctx context.Context, t *testing.T, at kubernetes.ApplicationTest) {
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
			},
		},
	})

	test.Test(t)
}
