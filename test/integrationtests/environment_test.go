// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package integrationtests

import (
	"context"
	"testing"

	azresources "github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/test/config"
	"github.com/Azure/radius/test/environment"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func TestAzureEnvironmentSetup(t *testing.T) {
	ctx := context.Background()

	config, err := config.NewAzureConfig()
	require.NoError(t, err, "failed to initialize azure config")

	// Find a test cluster
	env, err := environment.GetTestEnvironment(ctx, config)
	require.NoError(t, err)

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	resourceMap := make(map[string]string)
	resc := azresources.NewClient(env.SubscriptionID)
	resc.Authorizer = config.Authorizer

	for page, err := resc.ListByResourceGroup(ctx, env.ResourceGroup, "", "", nil); page.NotDone(); err = page.NextWithContext(ctx) {
		require.NoError(t, err, "failed to list resources")

		// Filter to the set of resources we deploy - this allows this test to run concurrently
		// with others.
		for _, r := range page.Values() {
			if radresources.HasRadiusEnvironmentTag(r.Tags) {
				resourceMap[*r.Type] = *r.ID
			}

			t.Logf("skipping non-environment resource: %s", *r.ID)
		}
	}

	// Check whether all the resources in the group are created
	require.Equal(t, 7, len(resourceMap), "Number of resources created by init step is unexpected")

	_, found := resourceMap["Microsoft.ContainerService/managedClusters"]
	require.True(t, found, "Microsoft.ContainerService/managedClusters resource not created")

	_, found = resourceMap["Microsoft.CustomProviders/resourceProviders"]
	require.True(t, found, "Microsoft.CustomProviders/resourceProviders resource not created")

	_, found = resourceMap["Microsoft.DocumentDB/databaseAccounts"]
	require.True(t, found, "Microsoft.DocumentDB/databaseAccounts resource not created")

	_, found = resourceMap["Microsoft.ManagedIdentity/userAssignedIdentities"]
	require.True(t, found, "Microsoft.ManagedIdentity/userAssignedIdentities resource not created")

	_, found = resourceMap["Microsoft.Web/serverFarms"]
	require.True(t, found, "Microsoft.Web/serverFarms resource not created")

	_, found = resourceMap["Microsoft.Web/sites"]
	require.True(t, found, "Microsoft.Web/sites resource not created")

	_, found = resourceMap["Microsoft.Resources/deploymentScripts"]
	require.True(t, found, "Microsoft.Resources/deploymentScripts resource not created")

	expectedPods := validation.PodSet{
		Namespaces: map[string][]validation.Pod{
			// verify dapr
			"dapr-system": {
				validation.Pod{Labels: map[string]string{"app": "dapr-dashboard"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-operator"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-placement-server"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-sentry"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-sidecar-injector"}},
			},
			// verify ingress-nginx
			"radius-system": {
				validation.Pod{Labels: map[string]string{"app.kubernetes.io/name": "ingress-nginx"}},
			},
		},
	}

	validation.ValidatePodsRunning(t, k8s, expectedPods)
}
