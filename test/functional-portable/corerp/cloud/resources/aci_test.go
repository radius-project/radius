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

package resource_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v9"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_ACI(t *testing.T) {
	name := "aci-app"
	containerResourceName := "frontend"
	containerResourceName2 := "magpie"
	gatewayResourceName := "gateway"
	template := "testdata/corerp-aci.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:             step.NewDeployExecutor(template).WithRetry(2, 60*time.Second, isTransientAzureError),
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
				// This ensures ACI resources use the resource group's location instead of a hardcoded one
				verifyACIResourceLocations(ctx, t, name)
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.Test(t)
}

// verifyACIResourceLocations verifies that ACI resources (VNet, NSG, Load Balancer) are created
// in the same region as the resource group, rather than a hardcoded location.
func verifyACIResourceLocations(ctx context.Context, t *testing.T, appName string) {
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
		resourceGroupName = fmt.Sprintf("radtest-%s", appName)
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

	// The environment name is hardcoded as "aci-env" in the Bicep template
	envName := "aci-env"

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

// transientAzureErrorMarkers are substrings that identify transient Azure
// deployment failures that may succeed on retry.
var transientAzureErrorMarkers = []string{
	// Managed identity propagation delay after environment identity creation.
	"ManagedServiceIdentityNotFound",
	// The regional ACI 'StandardCores' quota is shared across the subscription
	// and is frequently exhausted by concurrent CI runs and in-progress async
	// resource group cleanups. It drains on its own, so retrying after a short
	// delay typically succeeds.
	"ContainerGroupQuotaReached",
}

// isTransientAzureError returns true if the error is a known transient Azure
// error that may succeed on retry. It delegates to step.ErrorContainsAny, which
// flattens the nested ARM error details rad surfaces inside a CLIError so the
// match covers causes such as the ACI container group quota error.
func isTransientAzureError(err error) bool {
	return step.ErrorContainsAny(err, transientAzureErrorMarkers...)
}

func Test_isTransientAzureError(t *testing.T) {
	// aciQuotaError mirrors how rad surfaces an ACI quota failure: the quota
	// error code only appears inside a deeply nested details[].message field,
	// while the top-level code/message returned by CLIError.Error() is the
	// generic "DeploymentFailed".
	aciQuotaError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "At least one resource deployment operation failed.",
				Details: []*apiv1.ErrorDetails{
					{Code: "OK"},
					{
						Code:    "ResourceDeploymentFailure",
						Message: "Failed",
						Details: []*apiv1.ErrorDetails{
							{
								Code:    "Internal",
								Message: `ERROR CODE: AzureContainerInstance/ContainerGroupQuotaReached; container group quota 'StandardCores' exceeded in region 'westus2'. Limit: '10', Usage: '10' Requested: '1'.`,
							},
						},
					},
				},
			},
		},
	}

	// msiError mirrors a nested ManagedServiceIdentityNotFound failure, which is
	// also reported under a generic top-level "DeploymentFailed" code.
	msiError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "At least one resource deployment operation failed.",
				Details: []*apiv1.ErrorDetails{
					{
						Code:    "Internal",
						Message: "ManagedServiceIdentityNotFound: the managed identity could not be found",
					},
				},
			},
		},
	}

	nonTransientError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "the resource type is not supported",
			},
		},
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "nested ACI quota error", err: aciQuotaError, expected: true},
		{name: "nested managed identity error", err: msiError, expected: true},
		{name: "plain transient error string", err: errors.New("deployment failed: ManagedServiceIdentityNotFound"), expected: true},
		{name: "non-transient CLIError", err: nonTransientError, expected: false},
		{name: "unrelated error", err: errors.New("connection refused"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, isTransientAzureError(tc.err))
		})
	}
}
