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

package radius

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	backend_ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testrp"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiVersionParameter      = "api-version=2023-10-01-preview"
	testRadiusPlaneID        = "/planes/radius/test"
	testResourceNamespace    = "System.Test"
	testResourceGroupID      = testRadiusPlaneID + "/resourceGroups/test-rg"
	testResourceCollectionID = testResourceGroupID + "/providers/System.Test/testResources"
	testResourceID           = testResourceCollectionID + "/test-resource"

	assertTimeout = time.Second * 10
	assertRetry   = time.Second * 2
)

func Test_RadiusPlane_Proxy_ResourceGroupDoesNotExist(t *testing.T) {
	ucp := testserver.StartWithETCD(t, api.DefaultModules)
	rp := testrp.Start(t)

	rps := map[string]*string{
		testResourceNamespace: to.Ptr("http://" + rp.Address()),
	}
	createRadiusPlane(ucp, rps)

	response := ucp.MakeRequest(http.MethodGet, testResourceID, nil)
	response.EqualsErrorCode(http.StatusNotFound, "NotFound")
	require.Equal(t, "the resource with id '/planes/radius/test/resourceGroups/test-rg/providers/System.Test/testResources/test-resource' was not found", response.Error.Error.Message)
}

func Test_RadiusPlane_ResourceSync(t *testing.T) {
	ucp := testserver.StartWithETCD(t, api.DefaultModules)
	rp := testrp.Start(t)
	rp.Handler = testrp.SyncResource(t, ucp, testResourceGroupID)

	rps := map[string]*string{
		testResourceNamespace: to.Ptr("http://" + rp.Address()),
	}
	createRadiusPlane(ucp, rps)

	createResourceGroup(ucp, testResourceGroupID)

	message := "here is some test data"

	expectedTrackedResource := v20231001preview.GenericResource{
		ID:   to.Ptr(testResourceID),
		Name: to.Ptr("test-resource"),
		Type: to.Ptr("System.Test/testResources"),
	}

	t.Run("PUT", func(t *testing.T) {
		data := testrp.TestResource{
			Properties: testrp.TestResourceProperties{
				Message: to.Ptr(message),
			},
		}
		body, err := json.Marshal(data)
		require.NoError(t, err)

		response := ucp.MakeRequest(http.MethodPut, testResourceID+"?api-version="+testrp.Version, body)
		response.EqualsStatusCode(http.StatusOK)

		resource := &testrp.TestResource{}
		err = json.Unmarshal(response.Body.Bytes(), resource)
		require.NoError(t, err)
		require.Equal(t, message, *resource.Properties.Message)
	})

	t.Run("LIST", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceCollectionID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &testrp.TestResourceList{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, message, *resources.Value[0].Properties.Message)
	})

	t.Run("List - Tracked Resources", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &v20231001preview.GenericResourceListResult{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, expectedTrackedResource, *resources.Value[0])
	})

	t.Run("GET", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resource := &testrp.TestResource{}
		err := json.Unmarshal(response.Body.Bytes(), resource)
		require.NoError(t, err)
		require.Equal(t, message, *resource.Properties.Message)
	})

	t.Run("DELETE", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodDelete, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)
	})

	t.Run("GET (after delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusNotFound)
	})

	t.Run("List - Tracked Resources (after delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &v20231001preview.GenericResourceListResult{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Empty(t, resources.Value)
	})

	t.Run("DELETE (again)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodDelete, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusNoContent)
	})
}

func Test_RadiusPlane_ResourceAsync(t *testing.T) {
	ucp := testserver.StartWithETCD(t, api.DefaultModules)
	rp := testrp.Start(t)

	// Block background work item completion until we're ready.
	putCh := make(chan backend_ctrl.Result)
	deleteCh := make(chan backend_ctrl.Result)
	onPut := func(ctx context.Context, request *backend_ctrl.Request) (backend_ctrl.Result, error) {
		t.Log("PUT operation is waiting for completion")
		result := <-putCh
		return result, nil
	}
	onDelete := func(ctx context.Context, request *backend_ctrl.Request) (backend_ctrl.Result, error) {
		t.Log("DELETE operation is waiting for completion")
		result := <-deleteCh
		if result.Requeue || result.Error != nil {
			return result, nil
		}

		client, err := ucp.Clients.StorageProvider.GetStorageClient(ctx, "System.Test/testResources")
		require.NoError(t, err)
		err = client.Delete(ctx, testResourceID)
		require.NoError(t, err)

		return backend_ctrl.Result{}, nil
	}

	rp.Handler = testrp.AsyncResource(t, ucp, testResourceGroupID, onPut, onDelete)

	rps := map[string]*string{
		testResourceNamespace: to.Ptr("http://" + rp.Address()),
	}
	createRadiusPlane(ucp, rps)

	createResourceGroup(ucp, testResourceGroupID)

	message := "here is some test data"

	expectedTrackedResource := v20231001preview.GenericResource{
		ID:   to.Ptr(testResourceID),
		Name: to.Ptr("test-resource"),
		Type: to.Ptr("System.Test/testResources"),
	}

	t.Run("PUT", func(t *testing.T) {
		t.Log("starting PUT operation")
		data := testrp.TestResource{
			Properties: testrp.TestResourceProperties{
				Message: to.Ptr(message),
			},
		}
		body, err := json.Marshal(data)
		require.NoError(t, err)

		response := ucp.MakeRequest(http.MethodPut, testResourceID+"?api-version="+testrp.Version, body)
		response.EqualsStatusCode(http.StatusCreated)

		resource := &testrp.TestResource{}
		err = json.Unmarshal(response.Body.Bytes(), resource)
		require.NoError(t, err)
		require.Equal(t, message, *resource.Properties.Message)
		require.False(t, v1.ProvisioningState(*resource.Properties.ProvisioningState).IsTerminal())

		location := response.Raw.Header.Get("Location")
		azureAsyncOperation := response.Raw.Header.Get("Azure-AsyncOperation")
		require.True(t, strings.HasPrefix(location, ucp.BaseURL), "Location starts with UCP URL")
		require.True(t, strings.HasPrefix(azureAsyncOperation, ucp.BaseURL), "Azure-AsyncOperation starts with UCP URL")
	})

	t.Run("LIST (during PUT)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceCollectionID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &testrp.TestResourceList{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, message, *resources.Value[0].Properties.Message)
		require.False(t, v1.ProvisioningState(*resources.Value[0].Properties.ProvisioningState).IsTerminal())
	})

	t.Run("List - Tracked Resources (during PUT)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &v20231001preview.GenericResourceListResult{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, expectedTrackedResource, *resources.Value[0])
	})

	t.Run("GET (during PUT)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resource := &testrp.TestResource{}
		err := json.Unmarshal(response.Body.Bytes(), resource)
		require.NoError(t, err)
		require.Equal(t, message, *resource.Properties.Message)
		require.Equal(t, string(v1.ProvisioningStateAccepted), *resource.Properties.ProvisioningState)
	})

	t.Run("Complete PUT", func(t *testing.T) {
		t.Log("completing PUT operation")
		putCh <- backend_ctrl.Result{}
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
			assert.Equal(collect, http.StatusOK, response.Raw.StatusCode)

			resource := &testrp.TestResource{}
			err := json.Unmarshal(response.Body.Bytes(), resource)
			assert.NoError(collect, err)
			assert.Equal(collect, string(v1.ProvisioningStateSucceeded), *resource.Properties.ProvisioningState)
		}, assertTimeout, assertRetry)
	})

	t.Run("DELETE FAILURE", func(t *testing.T) {
		t.Log("starting DELETE FAILURE operation")
		response := ucp.MakeRequest(http.MethodDelete, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusAccepted)
	})

	t.Run("Complete DELETE FAILURE", func(t *testing.T) {
		t.Log("completing DELETE FAILURE operation")
		deleteCh <- backend_ctrl.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeInternal,
			Message: "Oh no!",
		})
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
			assert.Equal(collect, http.StatusOK, response.Raw.StatusCode)

			resource := &testrp.TestResource{}
			err := json.Unmarshal(response.Body.Bytes(), resource)
			assert.NoError(collect, err)
			assert.Equal(collect, string(v1.ProvisioningStateFailed), *resource.Properties.ProvisioningState)
			t.Logf("Resource provisioning state: %s", *resource.Properties.ProvisioningState)
		}, assertTimeout, assertRetry)
	})

	t.Run("List - Tracked Resources (after failed delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &v20231001preview.GenericResourceListResult{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, expectedTrackedResource, *resources.Value[0])
	})

	t.Run("DELETE", func(t *testing.T) {
		t.Log("starting DELETE operation")
		response := ucp.MakeRequest(http.MethodDelete, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusAccepted)
	})

	t.Run("LIST (during delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceCollectionID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &testrp.TestResourceList{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, message, *resources.Value[0].Properties.Message)
		require.False(t, v1.ProvisioningState(*resources.Value[0].Properties.ProvisioningState).IsTerminal())
	})

	t.Run("List - Tracked Resources (during delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resources := &v20231001preview.GenericResourceListResult{}
		err := json.Unmarshal(response.Body.Bytes(), resources)
		require.NoError(t, err)
		require.Len(t, resources.Value, 1)
		require.Equal(t, expectedTrackedResource, *resources.Value[0])
	})

	t.Run("GET (during delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusOK)

		resource := &testrp.TestResource{}
		err := json.Unmarshal(response.Body.Bytes(), resource)
		require.NoError(t, err)
		require.Equal(t, message, *resource.Properties.Message)
		require.False(t, v1.ProvisioningState(*resource.Properties.ProvisioningState).IsTerminal())
	})

	t.Run("Complete DELETE", func(t *testing.T) {
		t.Log("completing DELETE operation")
		deleteCh <- backend_ctrl.Result{}
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
			assert.Equal(collect, http.StatusNotFound, response.Raw.StatusCode)
		}, assertTimeout, assertRetry)
	})

	t.Run("GET (after delete)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodGet, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusNotFound)
	})

	t.Run("List - Tracked Resources (after delete)", func(t *testing.T) {
		// This is eventually consistent.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			response := ucp.MakeRequest(http.MethodGet, testResourceGroupID+"/resources?api-version="+v20231001preview.Version, nil)
			assert.Equal(collect, http.StatusOK, response.Raw.StatusCode)

			resources := &v20231001preview.GenericResourceListResult{}
			err := json.Unmarshal(response.Body.Bytes(), resources)
			assert.NoError(collect, err)
			assert.Empty(collect, resources.Value)
		}, assertTimeout, assertRetry)
	})

	t.Run("DELETE (again)", func(t *testing.T) {
		response := ucp.MakeRequest(http.MethodDelete, testResourceID+"?api-version="+testrp.Version, nil)
		response.EqualsStatusCode(http.StatusNoContent)
	})
}

func createRadiusPlane(ucp *testserver.TestServer, resourceProviders map[string]*string) {
	body := v20231001preview.PlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.PlaneResourceProperties{
			Kind:              to.Ptr(v20231001preview.PlaneKindUCPNative),
			ResourceProviders: resourceProviders,
		},
	}
	response := ucp.MakeTypedRequest(http.MethodPut, testRadiusPlaneID+"?"+apiVersionParameter, body)
	response.EqualsStatusCode(http.StatusOK)
}

func createResourceGroup(ucp *testserver.TestServer, id string) {
	body := v20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceGroupProperties{},
	}
	response := ucp.MakeTypedRequest(http.MethodPut, id+"?"+apiVersionParameter, body)
	response.EqualsStatusCode(http.StatusOK)
}
