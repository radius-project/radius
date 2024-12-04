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

package dynamic

import (
	"context"
	"net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp/testhost"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucptesthost "github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
)

const (
	radiusPlaneName           = "testing"
	resourceProviderNamespace = "Applications.Test"
	resourceTypeName          = "exampleResources"
	locationName              = v1.LocationGlobal
	apiVersion                = "2024-01-01"

	resourceGroupName   = "test-group"
	exampleResourceName = "my-example"

	exampleResourcePlaneID = "/planes/radius/" + radiusPlaneName
	exampleResourceGroupID = exampleResourcePlaneID + "/resourceGroups/test-group"

	exampleResourceID  = exampleResourceGroupID + "/providers/Applications.Test/exampleResources/" + exampleResourceName
	exampleResourceURL = exampleResourceID + "?api-version=" + apiVersion
)

// This test covers the lifecycle of a dynamic resource.
func Test_Dynamic_Resource_Lifecycle(t *testing.T) {
	_, ucp := testhost.Start(t)

	// Setup a resource provider (Applications.Test/exampleResources)
	createRadiusPlane(ucp)
	createResourceProvider(ucp)
	createResourceType(ucp)
	createAPIVersion(ucp)
	createLocation(ucp)

	// Setup a resource group where we can interact with the new resource type.
	createResourceGroup(ucp)

	// We have not yet implemented any functionality for dynamic RP.
	//
	// This is the hello-worldiest of tests. We're just making sure that all
	// of the infrastructure works.
	response := ucp.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsErrorCode(404, "NotFound")
}

func createRadiusPlane(server *ucptesthost.TestHost) v20231001preview.RadiusPlanesClientCreateOrUpdateResponse {
	ctx := context.Background()

	plane := v20231001preview.RadiusPlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.RadiusPlaneResourceProperties{
			// Note: this is a workaround. Properties is marked as a required field in
			// the API. Without passing *something* here the body will be rejected.
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
			ResourceProviders: map[string]*string{},
		},
	}

	client := server.UCP().NewRadiusPlanesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, plane, nil)
	require.NoError(server.T(), err)

	response, err := poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)

	return response
}

func createResourceProvider(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceProvider := v20231001preview.ResourceProviderResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceProviderProperties{},
	}

	client := server.UCP().NewResourceProvidersClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceProvider, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createResourceType(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceType := v20231001preview.ResourceTypeResource{
		Properties: &v20231001preview.ResourceTypeProperties{},
	}

	client := server.UCP().NewResourceTypesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceTypeName, resourceType, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createAPIVersion(server *ucptesthost.TestHost) {
	ctx := context.Background()

	apiVersionResource := v20231001preview.APIVersionResource{
		Properties: &v20231001preview.APIVersionProperties{},
	}

	client := server.UCP().NewAPIVersionsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceTypeName, apiVersion, apiVersionResource, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createLocation(server *ucptesthost.TestHost) {
	ctx := context.Background()

	location := v20231001preview.LocationResource{
		Properties: &v20231001preview.LocationProperties{
			ResourceTypes: map[string]*v20231001preview.LocationResourceType{
				resourceTypeName: {
					APIVersions: map[string]map[string]any{
						apiVersion: {},
					},
				},
			},
		},
	}

	client := server.UCP().NewLocationsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, locationName, location, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createResourceGroup(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceGroup := v20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceGroupProperties{},
	}

	client := server.UCP().NewResourceGroupsClient()
	_, err := client.CreateOrUpdate(ctx, radiusPlaneName, resourceGroupName, resourceGroup, nil)
	require.NoError(server.T(), err)
}
