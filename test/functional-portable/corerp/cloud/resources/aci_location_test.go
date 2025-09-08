/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package resource_test contains tests for ACI resource location.
// This test verifies that ACI resources (VNet, NSG, Load Balancer) are created
// in the same region as the resource group, rather than a hardcoded location.
//
// To run with Azure verification:
//   export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
//   export INTEGRATION_TEST_RESOURCE_GROUP_NAME=<resource-group-name>
//   go test -v -run Test_ACI_ResourceGroupLocation

package resource_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_ACI_ResourceGroupLocation(t *testing.T) {
	// This test verifies that ACI resources are created in the same location
	// as the resource group, not a hardcoded location
	name := "aci-location-test"
	containerResourceName := "aci-location-frontend"
	containerResourceName2 := "aci-location-magpie"
	gatewayResourceName := "aci-location-gateway"
	template := "testdata/corerp-aci-location.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template,
				fmt.Sprintf("basename=%s", name),
			),
			SkipObjectValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerResourceName,
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: containerResourceName2,
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: gatewayResourceName,
						Type: validation.GatewaysResource,
						App:  name,
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Verify that Azure resources are created in the correct location
				verifyResourceLocations(ctx, t, name)
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.Test(t)
}

func verifyResourceLocations(ctx context.Context, t *testing.T, baseName string) {
	// Get Azure subscription ID from environment
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		t.Log("AZURE_SUBSCRIPTION_ID not set, skipping Azure resource location verification")
		// Still mark test as passed since the deployment itself succeeded
		return
	}

	// Get the resource group name from CI environment or use default pattern
	resourceGroupName := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	if resourceGroupName == "" {
		// Fallback for local testing
		t.Log("INTEGRATION_TEST_RESOURCE_GROUP_NAME not set, using default pattern")
		resourceGroupName = fmt.Sprintf("radtest-%s", baseName)
	}

	t.Logf("Verifying resources in resource group: %s", resourceGroupName)

	// Create Azure credentials
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoError(t, err, "Failed to create Azure credentials")

	// Get resource group location first
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	require.NoError(t, err, "Failed to create resource groups client")

	rg, err := rgClient.Get(ctx, resourceGroupName, nil)
	require.NoError(t, err, "Failed to get resource group")
	require.NotNil(t, rg.Location, "Resource group location should not be nil")

	expectedLocation := normalizeLocation(*rg.Location)
	t.Logf("Resource group '%s' is in location: %s", resourceGroupName, *rg.Location)

	// Check network resources
	networkClient, err := armnetwork.NewClientFactory(subscriptionID, cred, nil)
	require.NoError(t, err, "Failed to create network client factory")

	// The environment name is used as the base for resource names
	envName := fmt.Sprintf("%s-env", baseName)

	// Check Virtual Network location (VNet name is same as env name)
	vnetName := envName
	vnet, err := networkClient.NewVirtualNetworksClient().Get(ctx, resourceGroupName, vnetName, nil)
	require.NoError(t, err, "Failed to get VNet %s: %v", vnetName, err)
	require.NotNil(t, vnet.Location, "VNet location should not be nil")
	actualVNetLocation := normalizeLocation(*vnet.Location)
	require.Equal(t, expectedLocation, actualVNetLocation,
		"VNet should be in the same location as resource group. Resource Group: %s, VNet: %s",
		*rg.Location, *vnet.Location)
	t.Logf("✓ VNet created in correct location: %s (same as resource group)", *vnet.Location)

	// Check Network Security Group location
	nsgName := fmt.Sprintf("%s-nsg", envName)
	nsg, err := networkClient.NewSecurityGroupsClient().Get(ctx, resourceGroupName, nsgName, nil)
	require.NoError(t, err, "Failed to get NSG %s: %v", nsgName, err)
	require.NotNil(t, nsg.Location, "NSG location should not be nil")
	actualNSGLocation := normalizeLocation(*nsg.Location)
	require.Equal(t, expectedLocation, actualNSGLocation,
		"NSG should be in the same location as resource group. Resource Group: %s, NSG: %s",
		*rg.Location, *nsg.Location)
	t.Logf("✓ NSG created in correct location: %s (same as resource group)", *nsg.Location)

	// Check Load Balancer location
	lbName := fmt.Sprintf("%s-ilb", envName)
	lb, err := networkClient.NewLoadBalancersClient().Get(ctx, resourceGroupName, lbName, nil)
	require.NoError(t, err, "Failed to get Load Balancer %s: %v", lbName, err)
	require.NotNil(t, lb.Location, "Load Balancer location should not be nil")
	actualLBLocation := normalizeLocation(*lb.Location)
	require.Equal(t, expectedLocation, actualLBLocation,
		"Load Balancer should be in the same location as resource group. Resource Group: %s, LB: %s",
		*rg.Location, *lb.Location)
	t.Logf("✓ Load Balancer created in correct location: %s (same as resource group)", *lb.Location)
}

// normalizeLocation normalizes Azure location strings for comparison
// e.g., "East US" -> "eastus", "West Europe" -> "westeurope"
func normalizeLocation(location string) string {
	return strings.ToLower(strings.ReplaceAll(location, " ", ""))
}
