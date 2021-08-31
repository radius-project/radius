// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment_test

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/azure/azclients"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/testcontext"
	"github.com/Azure/radius/test/validation"
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

		_, found = resources[azresources.ManagedIdentityUserAssignedIdentities]
		require.Truef(t, found, "%s resource not created", azresources.ManagedIdentityUserAssignedIdentities)

		_, found = resources[azresources.WebServerFarms]
		require.Truef(t, found, "%s resource not created", azresources.WebServerFarms)

		_, found = resources[azresources.WebSites]
		require.Truef(t, found, "%s resource not created", azresources.WebSites)

		// Currently, we have a retention policy on the deploymentScript for 1 day.
		// "retentionInterval": "P1D"
		// This means the script may or may not be present when checking the number of resources
		// if the environment was created over a day ago.
		// Verify that either 5 or 6 resources are present, and only check the deploymentScripts
		// if there are 6 resources
		if len(resources) == 6 {
			_, found = resources[azresources.ResourcesDeploymentScripts]
			require.Truef(t, found, "%s resource not created", azresources.ResourcesDeploymentScripts)
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
					validation.K8sObject{Labels: map[string]string{kubernetes.LabelName: "ingress-nginx"}},
				},
			},
		}

		validation.ValidatePodsRunning(ctx, t, options.K8sClient, expectedPods)
	})
}

func listRadiusEnvironmentResources(ctx context.Context, t *testing.T, env *environments.AzureCloudEnvironment, auth autorest.Authorizer, resourceGroup string) map[string]string {
	resourceMap := make(map[string]string)
	resc := azclients.NewResourcesClient(env.SubscriptionID, auth)

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
