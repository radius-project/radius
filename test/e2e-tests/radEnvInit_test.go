// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package itests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/test/config"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

// Radius env setup test
func TestAzureEnvironmentSetup(t *testing.T) {
	ctx := context.Background()

	config, err := config.NewAzureConfig()
	require.NoError(t, err, "failed to initialize azure config")
	require.NotEmpty(t, config.SubscriptionID(), "Subscription Id must be set via INTEGRATION_TEST_SUBSCRIPTION_ID")

	cwd, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	resourceGroupName := config.GenerateGroupName()
	t.Cleanup(func() {
		cleanup(ctx, t, config, resourceGroupName)
	})

	// use the local copy of the deployment template - this is needed for correctness when running as part of a PR
	deploymentTemplateFilePath := filepath.Join(cwd, "../../deploy/rp-full.json")
	require.FileExists(t, deploymentTemplateFilePath)

	// Run the rad cli init command and look for errors
	t.Log("Deploying in resource group: " + resourceGroupName)
	err = utils.RunRadInitCommand(config.SubscriptionID(), resourceGroupName, config.DefaultLocation(), deploymentTemplateFilePath, time.Minute*15)
	require.NoError(t, err)

	// Check whether the resource group is created
	groupc := resources.NewGroupsClient(config.SubscriptionID())
	groupc.Authorizer = config.Authorizer

	rg, err := groupc.Get(ctx, resourceGroupName)
	require.NoError(t, err, "failed to find resource group")
	require.Equal(t, resourceGroupName, *rg.Name)

	resourceMap := make(map[string]string)
	resc := resources.NewClient(config.SubscriptionID())
	resc.Authorizer = config.Authorizer

	for page, err := resc.ListByResourceGroup(ctx, resourceGroupName, "", "", nil); page.NotDone(); err = page.NextWithContext(ctx) {
		require.NoError(t, err, "failed to list resources")

		for _, r := range page.Values() {
			resourceMap[*r.Type] = *r.ID
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

	// Deploy bicep template
	templateFilePath := filepath.Join(cwd, "../../examples/frontend-backend/azure-bicep/template.bicep")
	require.FileExists(t, templateFilePath, "could not find application template")

	err = utils.RunRadDeployCommand(templateFilePath, "", time.Minute*5)
	require.NoError(t, err, "application deployment failed")

	// Merge the k8s credentials to the cluster
	err = utils.RunRadMergeCredentialsCommand("")
	require.NoError(t, err, "failed to run merge credentials")

	expectedPods := validation.PodSet{
		Namespaces: map[string][]validation.Pod{

			// verify app
			"frontend-backend": {
				validation.NewPodForComponent("frontend-backend", "frontend"),
				validation.NewPodForComponent("frontend-backend", "backend"),
			},

			// verify dapr
			"dapr-system": {
				validation.Pod{Labels: map[string]string{"app": "dapr-dashboard"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-operator"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-placement-server"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-sentry"}},
				validation.Pod{Labels: map[string]string{"app": "dapr-sidecar-injector"}},
			},
		},
	}

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	validation.ValidatePodsRunning(t, k8s, expectedPods)
}

func cleanup(ctx context.Context, t *testing.T, config *config.AzureConfig, resourceGroupName string) {
	groupc := resources.NewGroupsClient(config.SubscriptionID())
	groupc.Authorizer = config.Authorizer

	_, err := groupc.Delete(ctx, resourceGroupName)
	require.NoError(t, err, "failed to delete resource group")
}
