// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment_test

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func TestAzureEnvironment(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	options := azuretest.NewTestOptions(t)

	t.Run("Validate App Resource Group", func(t *testing.T) {
		resources := listRadiusEnvironmentResources(ctx, t, options.Environment, options.ARMAuthorizer, options.Environment.ResourceGroup)
		require.Equal(t, len(resources), 1, "Number of resources created by init step is less than expected")

		_, found := resources[azresources.CustomProvidersResourceProviders]
		require.Truef(t, found, "%s resource not created", azresources.CustomProvidersResourceProviders)
	})

	t.Run("Validate Control Plane Resource Group", func(t *testing.T) {
		resources := listRadiusEnvironmentResources(ctx, t, options.Environment, options.ARMAuthorizer, options.Environment.ControlPlaneResourceGroup)

		_, found := resources[azresources.ContainerServiceManagedClusters]
		require.Truef(t, found, "%s resource not created", azresources.ContainerServiceManagedClusters)

		_, found = resources[azresources.DocumentDBDatabaseAccounts]
		require.Truef(t, found, "%s resource not created", azresources.DocumentDBDatabaseAccounts)

		_, found = resources[azresources.WebServerFarms]
		require.Truef(t, found, "%s resource not created", azresources.WebServerFarms)

		_, found = resources[azresources.WebSites]
		require.Truef(t, found, "%s resource not created", azresources.WebSites)

		require.Equal(t, len(resources), 4, "Number of resources created by init step is greater than expected")

	})

	t.Run("Validate Kubernetes Runtime", func(t *testing.T) {
		expectedPods := validation.K8sObjectSet{
			Namespaces: map[string][]validation.K8sObject{
				"radius-system": {
					validation.NewK8sPodForResource("app", "dapr-dashboard"),
					validation.NewK8sPodForResource("app", "dapr-operator"),
					validation.NewK8sPodForResource("app", "dapr-placement-server"),
					validation.NewK8sPodForResource("app", "dapr-sentry"),
					validation.NewK8sPodForResource("app", "dapr-sidecar-injector"),
					validation.NewK8sPodForResource("app", "haproxy-ingress"),
				},
			},
		}

		validation.ValidateObjectsRunning(ctx, t, options.K8sClient, options.DynamicClient, expectedPods)
	})
}

func listRadiusEnvironmentResources(ctx context.Context, t *testing.T, env *environments.AzureCloudEnvironment, auth autorest.Authorizer, resourceGroup string) map[string]string {
	resourceMap := make(map[string]string)
	resc := clients.NewResourcesClient(env.SubscriptionID, auth)

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
