// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"testing"

	"./iam"

	"./config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureRadiusEnvInitialization(t *testing.T) {
	ctx := context.Background()

	// Read the test configuration from environment variables
	config.Read()

	resourceGroupName := azurehelpers.GenerateGroupName(config.BaseGroupName())
	radInitCmd := fmt.SPrintf("rad env init azure --resource-group %s --subscriptionId %s", resourceGroupName, config.SubscriptionID())
	cmd := exec.Command(radInitCmd)

	err := cmd.Run()

	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	// Check whether the resource group is created
	rg, err := GetGroup(ctx)
	if err != nil || rg.ID != config.GroupName() {
		log.Fatal(err)
	}
	resourceMap := make(map[string]string)
	authorizer, err := iam.GetResourceManagementAuthorizer()
	if err != nil {
		log.Fatalf("failed to initialize authorizer: %v\n", err)
	}

	for list, err := azurehelpers.ListResourcesInResourceGroup(config.SubscriptionID(), config.UserAgent(), config.GroupName(), authorizer); list.NotDone(); err = list.NextWithContext(ctx) {
		resourceMap[*list.Value().Type] = *list.Value().ID
	}

	assert.Equal(t, 6, len(resourceMap), "Number of resources created by init step is unexpected")

	_, found := resourceMap["Microsoft.ContainerService/managedClusters"]
	assert.True(t, found, "Microsoft.ContainerService/managedClusters resource not created")

	_, found = resourceMap["Microsoft.CustomProviders/resourceProviders"]
	assert.True(t, found, "Microsoft.CustomProviders/resourceProviders resource not created")

	_, found = resourceMap["Microsoft.DocumentDB/databaseAccounts"]
	assert.True(t, found, "Microsoft.DocumentDB/databaseAccounts resource not created")

	_, found = resourceMap["Microsoft.ManagedIdentity/userAssignedIdentities"]
	assert.True(t, found, "Microsoft.ManagedIdentity/userAssignedIdentities resource not created")

	_, found = resourceMap["Microsoft.Web/serverFarms"]
	assert.True(t, found, "Microsoft.Web/serverFarms resource not created")

	_, found = resourceMap["Microsoft.Web/sites"]
	assert.True(t, found, "Microsoft.Web/sites resource not created")

}
