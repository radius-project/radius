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

package resourcegroups

import (
	"net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	radiusAPIVersion           = "?api-version=2023-10-01-preview"
	radiusPlaneResourceURL     = "/planes/radius/local" + radiusAPIVersion
	radiusPlaneRequestFixture  = "../planes/testdata/radiusplane_v20231001preview_requestbody.json"
	radiusPlaneResponseFixture = "../planes/testdata/radiusplane_v20231001preview_responsebody.json"

	resourceProviderCollectionURL = "/planes/radius/local/providers" + radiusAPIVersion

	resourceProviderNamespace = "Applications.Test"
	resourceProviderID        = "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test"
	resourceProviderURL       = "/planes/radius/local/providers/" + resourceProviderNamespace + radiusAPIVersion

	exampleResourceGroupID       = "/planes/radius/local/resourceGroups/test-group"
	exampleResourceCollectionURL = exampleResourceGroupID + "/providers/Applications.Test/exampleResources" + exampleResourceAPIVersion

	exampleResourceName       = "my-example"
	exampleResourceID         = exampleResourceGroupID + "/providers/Applications.Test/exampleResources/" + exampleResourceName
	exampleResourceAPIVersion = "?api-version=2024-01-01"
	exampleResourceURL        = exampleResourceID + exampleResourceAPIVersion

	resourceProviderEmptyListResponseFixture = "testdata/resourceprovider_v20231001preview_emptylist_responsebody.json"
	resourceProviderListResponseFixture      = "testdata/resourceprovider_v20231001preview_list_responsebody.json"

	resourceProviderRequestFixture  = "testdata/resourceprovider_v20231001preview_requestbody.json"
	resourceProviderResponseFixture = "testdata/resourceprovider_v20231001preview_responsebody.json"

	exampleResourceEmptyListResponseFixture = "testdata/exampleresource_v20240101preview_emptylist_responsebody.json"
	exampleResourceListResponseFixture      = "testdata/exampleresource_v20240101preview_list_responsebody.json"

	exampleResourceRequestFixture          = "testdata/exampleresource_v20240101preview_requestbody.json"
	exampleResourceResponseFixture         = "testdata/exampleresource_v20240101preview_responsebody.json"
	exampleResourceAcceptedResponseFixture = "testdata/exampleresource_v20240101preview_accepted_responsebody.json"
)

func createRadiusPlane(server *testserver.TestServer) {
	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.EqualsFixture(200, radiusPlaneResponseFixture)
}

func createResourceProvider(server *testserver.TestServer) {
	response := server.MakeFixtureRequest("PUT", resourceProviderURL, resourceProviderRequestFixture)
	response.EqualsFixture(200, resourceProviderResponseFixture)
}

func createResourceGroup(server *testserver.TestServer) {
	body := v20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceGroupProperties{},
	}
	response := server.MakeTypedRequest(http.MethodPut, exampleResourceGroupID+radiusAPIVersion, body)
	response.EqualsStatusCode(http.StatusOK)
}

func Test_ResourceProvider_Lifecycle(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	createRadiusPlane(server)

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// List should start empty
	response := server.MakeRequest(http.MethodGet, resourceProviderCollectionURL, nil)
	response.EqualsFixture(200, resourceProviderEmptyListResponseFixture)

	// Getting a specific resource provider should return 404 with the correct resource ID.
	response = server.MakeRequest(http.MethodGet, resourceProviderURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, resourceProviderID, response.Error.Error.Target)

	// Create a resource provider
	createResourceProvider(server)

	// List should now contain the resource provider
	response = server.MakeRequest(http.MethodGet, resourceProviderCollectionURL, nil)
	response.EqualsFixture(200, resourceProviderListResponseFixture)

	// Getting the resource provider should return 200
	response = server.MakeRequest(http.MethodGet, resourceProviderURL, nil)
	response.EqualsFixture(200, resourceProviderResponseFixture)

	// Deleting a resource provider should return 200
	response = server.MakeRequest(http.MethodDelete, resourceProviderURL, nil)
	response.EqualsStatusCode(200)
}

func Test_ResourceProvider_Resource_Lifecycle(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// Setup a resource provider (Applications.Test/exampleResources)
	createRadiusPlane(server)
	createResourceProvider(server)
	createResourceGroup(server)

	// List should start empty
	response := server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceEmptyListResponseFixture)

	// Getting a specific resource should return 404.
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsErrorCode(404, "NotFound")

	// Create a resource
	response = server.MakeFixtureRequest(http.MethodPut, exampleResourceURL, exampleResourceRequestFixture)
	response.EqualsFixture(201, exampleResourceAcceptedResponseFixture)

	// Verify async operations
	operationStatusResponse := server.MakeRequest(http.MethodGet, response.Raw.Header.Get("Azure-AsyncOperation"), nil)
	operationStatusResponse.EqualsStatusCode(200)

	operationStatus := v1.AsyncOperationStatus{}
	operationStatusResponse.ReadAs(&operationStatus)

	require.Equal(t, v1.ProvisioningStateAccepted, operationStatus.Status)
	require.NotNil(t, operationStatus.StartTime)

	statusID, err := resources.ParseResource(operationStatus.ID)
	require.NoError(t, err)
	require.Equal(t, "applications.test/locations/operationstatuses", statusID.Type())
	require.Equal(t, statusID.Name(), operationStatus.Name)

	operationResultResponse := server.MakeRequest(http.MethodGet, response.Raw.Header.Get("Location"), nil)
	require.Truef(t, operationResultResponse.Raw.StatusCode == http.StatusAccepted || operationResultResponse.Raw.StatusCode == http.StatusNoContent, "Expected 202 or 204 response")

	response = response.WaitForOperationComplete(nil)
	response.EqualsStatusCode(200)

	// List should now contain the resource
	response = server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceListResponseFixture)

	// Getting the resource should return 200
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsFixture(200, exampleResourceResponseFixture)

	// Deleting a resource should return 200
	response = server.MakeRequest(http.MethodDelete, exampleResourceURL, nil)
	response.EqualsFixture(202, exampleResourceAcceptedResponseFixture)
	response = response.WaitForOperationComplete(nil)
	response.EqualsStatusCode(200)

	// Now the resource is gone
	response = server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceEmptyListResponseFixture)
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsErrorCode(404, "NotFound")
}
