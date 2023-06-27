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

package integrationtests

// Tests that test with Mock RP functionality and UCP Server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	"github.com/project-radius/radius/pkg/ucp/frontend/radius"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	corerpURL                 = "127.0.0.1:7443"
	msgrpURL                  = "127.0.0.1:7442"
	azureURL                  = "127.0.0.1:9443"
	testProxyRequestCorePath  = "/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications"
	testProxyRequestMsgPath   = "/planes/radius/local/resourceGroups/rg1/providers/Applications.Messaging/rabbitMQQueues"
	testProxyRequestAzurePath = "/subscriptions/sid/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1"
	apiVersionQueyParam       = "api-version=2022-09-01-privatepreview"
	testUCPNativePlaneID      = "/planes/radius/local"
	testAzurePlaneID          = "/planes/azure/azurecloud"
	pathBase                  = "/apis/api.ucp.dev/v1alpha3"
)

var planeKindAzure v20220901privatepreview.PlaneKind = v20220901privatepreview.PlaneKindAzure
var applicationList = []map[string]any{
	{
		"Name": "app1",
	},
	{
		"Name": "app2",
	},
}

var testCoreUCPNativePlane = datamodel.Plane{
	BaseResource: v1.BaseResource{
		TrackedResource: v1.TrackedResource{
			ID:   "/planes/radius/local",
			Type: "radius",
			Name: "local",
		},
	},
	Properties: datamodel.PlaneProperties{
		Kind: rest.PlaneKindUCPNative,
		ResourceProviders: map[string]*string{
			"Applications.Core": to.Ptr("http://" + corerpURL),
		},
	},
}

var testMsgUCPNativePlane = datamodel.Plane{
	BaseResource: v1.BaseResource{
		TrackedResource: v1.TrackedResource{
			ID:   "/planes/radius/local",
			Type: "radius",
			Name: "local",
		},
	},
	Properties: datamodel.PlaneProperties{
		Kind: rest.PlaneKindUCPNative,
		ResourceProviders: map[string]*string{
			"Applications.Messaging": to.Ptr("http://" + msgrpURL),
		},
	},
}

var testCoreUCPNativePlaneVersioned = v20220901privatepreview.PlaneResource{
	ID:   to.Ptr("/planes/radius/local"),
	Type: to.Ptr("System.Planes/radius"),
	Name: to.Ptr("local"),
	Properties: &v20220901privatepreview.PlaneResourceProperties{
		Kind: to.Ptr(v20220901privatepreview.PlaneKindUCPNative),
		ResourceProviders: map[string]*string{
			"Applications.Core": to.Ptr("http://" + corerpURL),
		},
	},
}

var testMsgUCPNativePlaneVersioned = v20220901privatepreview.PlaneResource{
	ID:   to.Ptr("/planes/radius/local"),
	Type: to.Ptr("System.Planes/radius"),
	Name: to.Ptr("local"),
	Properties: &v20220901privatepreview.PlaneResourceProperties{
		Kind: to.Ptr(v20220901privatepreview.PlaneKindUCPNative),
		ResourceProviders: map[string]*string{
			"Applications.Messaging": to.Ptr("http://" + msgrpURL),
		},
	},
}

var testAzurePlane = v20220901privatepreview.PlaneResource{
	ID:   to.Ptr(testAzurePlaneID),
	Name: to.Ptr("azurecloud"),
	Type: to.Ptr("System.Planes/azure"),
	Properties: &v20220901privatepreview.PlaneResourceProperties{
		Kind: &planeKindAzure,
		URL:  to.Ptr("http://" + azureURL),
	},
}

var testResourceGroup = v20220901privatepreview.ResourceGroupResource{
	ID:       to.Ptr(testUCPNativePlaneID + "/resourcegroups/rg1"),
	Name:     to.Ptr("rg1"),
	Type:     to.Ptr(resourcegroups.ResourceGroupType),
	Location: to.Ptr(v1.LocationGlobal),
	Tags:     map[string]*string{},
}

func Test_ProxyToRP(t *testing.T) {
	ucp, db, _ := testserver.StartWithMocks(t, func(options modules.Options) []modules.Initializer {
		return []modules.Initializer{radius.NewModule(options, "radius")}
	})

	body, err := json.Marshal(applicationList)
	require.NoError(t, err)

	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, strings.ToLower(testProxyRequestCorePath), strings.ToLower(r.URL.Path))
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", ucp.BaseURL+testProxyRequestCorePath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", corerpURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	// Register RP with UCP
	registerRP(t, ucp, db, true, "Core")

	// Create a Resource group
	createResourceGroup(t, ucp, db)

	// Send a request that will be proxied to the RP
	sendProxyRequest(t, ucp, db, "Core")
}

func Test_ProxyToMessagingRP(t *testing.T) {
	ucp, db, _ := testserver.StartWithMocks(t, api.DefaultModules)

	body, err := json.Marshal(applicationList)
	require.NoError(t, err)

	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, strings.ToLower(testProxyRequestMsgPath), strings.ToLower(r.URL.Path))
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", ucp.BaseURL+testProxyRequestMsgPath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", msgrpURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	// Register RP with UCP
	registerRP(t, ucp, db, true, "Messaging")

	// Create a Resource group
	createResourceGroup(t, ucp, db)

	// Send a request that will be proxied to the RP
	sendProxyRequest(t, ucp, db, "Messaging")
}

func Test_ProxyToRP_NonNativePlane(t *testing.T) {
	ucp, db, _ := testserver.StartWithMocks(t, api.DefaultModules)

	body, err := json.Marshal(applicationList)
	require.NoError(t, err)
	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, strings.ToLower(testProxyRequestAzurePath), strings.ToLower(r.URL.Path))
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", "http://"+azureURL+testProxyRequestAzurePath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", azureURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	// Register RP with UCP
	registerRP(t, ucp, db, false, "Core")

	// Create a Resource group
	createResourceGroup(t, ucp, db)

	// Send a request that will be proxied to the RP
	sendProxyRequest_AzurePlane(t, ucp, db)
}

func Test_ProxyToRP_ResourceGroupDoesNotExist(t *testing.T) {
	ucp, db, _ := testserver.StartWithMocks(t, api.DefaultModules)

	body, err := json.Marshal(applicationList)
	require.NoError(t, err)
	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", "http://"+corerpURL+testProxyRequestCorePath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", corerpURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	// Register RP with UCP
	registerRP(t, ucp, db, true, "Core")

	// Send a request that will be proxied to the RP
	sendProxyRequest_ResourceGroupDoesNotExist(t, ucp, db)
}

func Test_MethodNotAllowed(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	// Send a request that will be proxied to the RP
	request, err := http.NewRequest("DELETE", ucp.BaseURL+"/planes", nil)
	require.NoError(t, err)
	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
}

func Test_NotFound(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	// Send a request that will be proxied to the RP
	request, err := http.NewRequest("GET", ucp.BaseURL+"/abc", nil)
	require.NoError(t, err)
	response, err := ucp.Client().Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, response.StatusCode)
}

func Test_APIValidationIsApplied(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	// Send a request that will be proxied to the RP
	requestBody := v20220901privatepreview.ResourceGroupResource{
		Tags: map[string]*string{},
		// Missing location
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	createResourceGroupRequest, err := http.NewRequest("PUT", ucp.BaseURL+"/planes/radius/local/resourcegroups/rg1?api-version=2022-09-01-privatepreview", bytes.NewBuffer(body))
	require.NoError(t, err)

	response, err := ucp.Client().Do(createResourceGroupRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func registerRP(t *testing.T, ucp *testserver.TestServer, db *store.MockStorageClient, ucpNative bool, rp string) {
	var requestBody map[string]any
	if ucpNative && rp == "Core" {
		requestBody = map[string]any{
			"location": v1.LocationGlobal,
			"properties": map[string]any{
				"resourceProviders": map[string]string{
					"Applications.Core": "http://" + corerpURL,
				},
				"kind": rest.PlaneKindUCPNative,
			},
		}
	} else if ucpNative && rp == "Messaging" {
		requestBody = map[string]any{
			"location": v1.LocationGlobal,
			"properties": map[string]any{
				"resourceProviders": map[string]string{
					"Applications.Messaging": "http://" + msgrpURL,
				},
				"kind": rest.PlaneKindUCPNative,
			},
		}

	} else {
		requestBody = map[string]any{
			"location": v1.LocationGlobal,
			"properties": map[string]any{
				"kind": rest.PlaneKindAzure,
				"url":  "http://" + azureURL,
			},
		}
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	var createPlaneRequest *http.Request
	if ucpNative {
		createPlaneRequest, err = rpctest.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodPut, ucp.BaseURL+"/planes/radius/local?api-version=2022-09-01-privatepreview", body)
	} else {
		createPlaneRequest, err = rpctest.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodPut, ucp.BaseURL+"/planes/azure/azurecloud?api-version=2022-09-01-privatepreview", body)
	}
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	db.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any())

	response, err := ucp.Client().Do(createPlaneRequest)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, response.StatusCode)

	defer response.Body.Close()
	registerPlaneResponseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	responsePlane := v20220901privatepreview.PlaneResource{}
	err = json.Unmarshal(registerPlaneResponseBody, &responsePlane)
	require.NoError(t, err)
	if ucpNative && rp == "Core" {
		require.Equal(t, testCoreUCPNativePlaneVersioned, responsePlane)
	} else if ucpNative && rp == "Messaging" {
		require.Equal(t, testMsgUCPNativePlaneVersioned, responsePlane)
	} else {
		require.Equal(t, testAzurePlane, responsePlane)
	}
}

func createResourceGroup(t *testing.T, ucp *testserver.TestServer, db *store.MockStorageClient) {
	requestBody := v20220901privatepreview.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
		Tags:     map[string]*string{},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	db.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any())
	createResourceGroupRequest, err := rpctest.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodPut, ucp.BaseURL+"/planes/radius/local/resourcegroups/rg1?api-version=2022-09-01-privatepreview", body)
	require.NoError(t, err)
	createResourceGroupResponse, err := ucp.Client().Do(createResourceGroupRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, createResourceGroupResponse.StatusCode)

	defer createResourceGroupResponse.Body.Close()
	createResourceGroupResponseBody, err := io.ReadAll(createResourceGroupResponse.Body)
	require.NoError(t, err)

	var responseResourceGroup v20220901privatepreview.ResourceGroupResource
	err = json.Unmarshal(createResourceGroupResponseBody, &responseResourceGroup)
	require.NoError(t, err)
	require.Equal(t, testResourceGroup, responseResourceGroup)
}

func sendProxyRequest(t *testing.T, ucp *testserver.TestServer, db *store.MockStorageClient, rp string) {
	var testProxyRequestPath string
	if rp == "Core" {
		testProxyRequestPath = testProxyRequestCorePath
		db.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return &store.Object{
				Metadata: store.Metadata{},
				Data:     &testCoreUCPNativePlane,
			}, nil
		})
	} else {
		testProxyRequestPath = testProxyRequestMsgPath
		db.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
			return &store.Object{
				Metadata: store.Metadata{},
				Data:     &testMsgUCPNativePlane,
			}, nil
		})
	}

	rgID, err := resources.ParseScope("/planes/radius/local/resourcegroups/rg1")
	require.NoError(t, err)
	db.EXPECT().Get(gomock.Any(), rgID.String()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &testResourceGroup,
		}, nil
	})

	proxyRequest, err := rpctest.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodPut, ucp.BaseURL+testProxyRequestPath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucp.Client().Do(proxyRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, proxyRequestResponse.StatusCode)
	require.Equal(t, apiVersionQueyParam, proxyRequestResponse.Request.URL.RawQuery)
	require.Equal(t, ucp.BaseURL+testProxyRequestPath, proxyRequestResponse.Header["Location"][0])

	defer proxyRequestResponse.Body.Close()
	proxyRequestResponseBody, err := io.ReadAll(proxyRequestResponse.Body)
	require.NoError(t, err)
	responseAppList := []map[string]any{}
	err = json.Unmarshal(proxyRequestResponseBody, &responseAppList)
	require.NoError(t, err)
	require.Equal(t, applicationList, responseAppList)
}

func sendProxyRequest_AzurePlane(t *testing.T, ucp *testserver.TestServer, db *store.MockStorageClient) {
	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		data := store.Object{
			Metadata: store.Metadata{},
			Data:     testAzurePlane,
		}
		return &data, nil
	})

	proxyRequest, err := rpctest.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodGet, ucp.BaseURL+"/planes/azure/azurecloud"+testProxyRequestAzurePath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucp.Client().Do(proxyRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, proxyRequestResponse.StatusCode)
	require.Equal(t, apiVersionQueyParam, proxyRequestResponse.Request.URL.RawQuery)

	defer proxyRequestResponse.Body.Close()
	proxyRequestResponseBody, err := io.ReadAll(proxyRequestResponse.Body)
	require.NoError(t, err)
	responseAppList := []map[string]any{}
	err = json.Unmarshal(proxyRequestResponseBody, &responseAppList)
	require.NoError(t, err)
	require.Equal(t, applicationList, responseAppList)
}

func sendProxyRequest_ResourceGroupDoesNotExist(t *testing.T, ucp *testserver.TestServer, db *store.MockStorageClient) {
	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		data := store.Object{
			Metadata: store.Metadata{},
			Data:     &testCoreUCPNativePlane,
		}
		return &data, nil
	})

	rgID, err := resources.ParseScope("/planes/radius/local/resourcegroups/rg1")
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), rgID.String()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	proxyRequest, err := http.NewRequest("GET", ucp.BaseURL+testProxyRequestCorePath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucp.Client().Do(proxyRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, proxyRequestResponse.StatusCode)
}

func Test_RequestWithBadAPIVersion(t *testing.T) {
	ucp, _, _ := testserver.StartWithMocks(t, api.DefaultModules)

	requestBody := map[string]any{
		"location": v1.LocationGlobal,
		"properties": map[string]any{
			"resourceProviders": map[string]string{
				"Applications.Core": "http://" + corerpURL,
			},
			"kind": rest.PlaneKindUCPNative,
		},
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, ucp.BaseURL+"/planes/radius/local?api-version=unsupported-version", bytes.NewBuffer(body))
	require.NoError(t, err)

	response, err := ucp.Client().Do(request)
	require.NoError(t, err)

	expectedResponse := v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    "InvalidApiVersionParameter",
			Message: "API version 'unsupported-version' for type 'ucp/openapi' is not supported. The supported api-versions are '2022-09-01-privatepreview'.",
		},
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var errorResponse v1.ErrorResponse
	err = json.Unmarshal(responseBody, &errorResponse)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, errorResponse)

}
