// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package itests

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/radius/test/e2e-tests/config"
	"github.com/Azure/radius/test/e2e-tests/utils"
	"github.com/stretchr/testify/require"
)

// Radius env setup test
func TestAzureEnvironmentSetup(t *testing.T) {
	ctx := context.Background()
	resourceGroupName := config.AzureConfig.GenerateGroupName()

	defer cleanup(ctx, resourceGroupName)

	// Run the rad cli init command and look for errors
	fmt.Println("Deploying in resource group: " + resourceGroupName)
	err := utils.RunRadInitCommand(config.AzureConfig.SubscriptionID(), resourceGroupName, config.AzureConfig.DefaultLocation(), time.Minute*15)
	if err != nil {
		fmt.Println(err)
	}
	require.NoError(t, err)

	// Check whether the resource group is created
	rg, err := utils.GetGroup(ctx, resourceGroupName)
	if err != nil || *rg.Name != resourceGroupName {
		log.Fatal(err)
	}
	resourceMap := make(map[string]string)

	for pageResults, _ := utils.ListResourcesInResourceGroup(ctx, resourceGroupName); pageResults.NotDone(); err = pageResults.NextWithContext(ctx) {
		if err != nil {
			log.Fatal(err)
			return
		}
		for _, r := range pageResults.Values() {
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
	cwd, _ := os.Getwd()
	templateFilePath := filepath.Join(cwd, "../frontend-backend/azure-bicep/template.bicep")
	err = utils.RunRadDeployCommand(templateFilePath, time.Minute*5)
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	// Merge the k8s credentials to the cluster
	err = utils.RunRadMergeCredentialsCommand()
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	expectedPods := make(map[string]int)
	// Validate dapr is installed and running
	expectedPods["dapr-system"] = 5
	// Validate pods specified in frontend-backend template are up and running
	expectedPods["frontend-backend"] = 2
	require.True(t, utils.ValidatePodsRunning(t, expectedPods))
}

func cleanup(ctx context.Context, resourceGroupName string) {
	_, err := utils.DeleteGroup(ctx, resourceGroupName)
	if err != nil {
		log.Fatal(err)
	}
}
