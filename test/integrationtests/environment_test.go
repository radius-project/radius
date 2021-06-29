// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package integrationtests

import (
	"context"
	"testing"

	azresources "github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func TestAzureEnvironmentSetup(t *testing.T) {
	ctx, cancel := utils.GetContext(t)
	defer cancel()

	config, err := rad.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	auth, err := azure.GetResourceManagerEndpointAuthorizer()
	require.NoError(t, err, "failed to authenticate with azure")

	env, err := rad.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	az, err := environments.RequireAzureCloud(env)
	require.NoError(t, err, "environment was not azure cloud")

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	t.Run("Validate App Resource Group", func(t *testing.T) {

		resources := listRadiusEnvironmentResources(ctx, t, az, auth, az.ControlPlaneResourceGroup)
		require.Equal(t, len(resources), 1, "Number of resources created by init step is less than expected")

		_, found := resources["Microsoft.CustomProviders/resourceProviders"]
		require.True(t, found, "Microsoft.CustomProviders/resourceProviders resource not created")
	})

	t.Run("Validate Control Plane Resource Group", func(t *testing.T) {
		resources := listRadiusEnvironmentResources(ctx, t, az, auth, az.ControlPlaneResourceGroup)

		_, found := resources["Microsoft.ContainerService/managedClusters"]
		require.True(t, found, "Microsoft.ContainerService/managedClusters resource not created")

		_, found = resources["Microsoft.DocumentDB/databaseAccounts"]
		require.True(t, found, "Microsoft.DocumentDB/databaseAccounts resource not created")

		_, found = resources["Microsoft.ManagedIdentity/userAssignedIdentities"]
		require.True(t, found, "Microsoft.ManagedIdentity/userAssignedIdentities resource not created")

		_, found = resources["Microsoft.Web/serverFarms"]
		require.True(t, found, "Microsoft.Web/serverFarms resource not created")

		_, found = resources["Microsoft.Web/sites"]
		require.True(t, found, "Microsoft.Web/sites resource not created")

		// Currently, we have a retention policy on the deploymentScript for 1 day.
		// "retentionInterval": "P1D"
		// This means the script may or may not be present when checking the number of resources
		// if the environment was created over a day ago.
		// Verify that either 5 or 6 resources are present, and only check the deploymentScripts
		// if there are 6 resources
		if len(resources) == 6 {
			_, found = resources["Microsoft.Resources/deploymentScripts"]
			require.True(t, found, "Microsoft.Resources/deploymentScripts resource not created")
		}

		require.GreaterOrEqual(t, len(resources), 5, "Number of resources created by init step is less than expected")
		require.LessOrEqual(t, len(resources), 6, "Number of resources created by init step is greater than expected")

	})

	t.Run("Validate Kubernetes Runtime", func(t *testing.T) {
		expectedPods := validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				// verify dapr
				"dapr-system": {
					validation.K8sObject{Labels: map[string]string{"app": "dapr-dashboard"}},
					validation.K8sObject{Labels: map[string]string{"app": "dapr-operator"}},
					validation.K8sObject{Labels: map[string]string{"app": "dapr-placement-server"}},
					validation.K8sObject{Labels: map[string]string{"app": "dapr-sentry"}},
					validation.K8sObject{Labels: map[string]string{"app": "dapr-sidecar-injector"}},
				},
				// verify ingress-nginx
				"radius-system": {
					validation.K8sObject{Labels: map[string]string{keys.LabelKubernetesName: "ingress-nginx"}},
				},
			},
		}

		validation.ValidatePodsRunning(ctx, t, k8s, expectedPods)
	})
}

func listRadiusEnvironmentResources(ctx context.Context, t *testing.T, env *environments.AzureCloudEnvironment, auth autorest.Authorizer, resourceGroup string) map[string]string {
	resourceMap := make(map[string]string)
	resc := azresources.NewClient(env.SubscriptionID)
	resc.Authorizer = auth

	for page, err := resc.ListByResourceGroup(ctx, resourceGroup, "", "", nil); page.NotDone(); err = page.NextWithContext(ctx) {
		require.NoError(t, err, "failed to list resources")

		// Filter to the set of resources we deploy - this allows this test to run concurrently
		// with others.
		for _, r := range page.Values() {
			if keys.HasRadiusEnvironmentTag(r.Tags) {
				resourceMap[*r.Type] = *r.ID
				t.Logf("environment resource: %s", *r.ID)
			} else {
				t.Logf("skipping non-environment resource: %s", *r.ID)
			}
		}
	}

	return resourceMap
}
