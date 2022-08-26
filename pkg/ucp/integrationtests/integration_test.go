// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(httpClient *http.Client, baseURL string) Client {
	return Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

const (
	rpURL                     = "127.0.0.1:7443"
	azureURL                  = "127.0.0.1:9443"
	testProxyRequestPath      = "/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications"
	testProxyRequestAzurePath = "/subscriptions/sid/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1"
	apiVersionQueyParam       = "api-version=2022-09-01-privatepreview"
	testUCPNativePlaneID      = "/planes/radius/local"
	testAzurePlaneID          = "/planes/azure/azurecloud"
	basePath                  = "/apis/api.ucp.dev/v1alpha3"
)

var planeKindUCPNative v20220901privatepreview.PlaneKind = v20220901privatepreview.PlaneKindUCPNative
var planeKindAzure v20220901privatepreview.PlaneKind = v20220901privatepreview.PlaneKindAzure
var applicationList = []map[string]interface{}{
	{
		"Name": "app1",
	},
	{
		"Name": "app2",
	},
}

var testUCPNativePlane = v20220901privatepreview.PlaneResource{
	ID:   to.Ptr(testUCPNativePlaneID),
	Name: to.Ptr("local"),
	Type: to.Ptr("System.Planes/radius"),
	Properties: &v20220901privatepreview.PlaneResourceProperties{
		ResourceProviders: map[string]*string{
			"Applications.Core": to.Ptr("http://" + rpURL),
		},
		Kind: &planeKindUCPNative,
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
	Name: to.Ptr("rg1"),
	ID:   to.Ptr(testUCPNativePlaneID + "/resourceGroups/rg1"),
	Type: to.Ptr(""),
}

func Test_ProxyToRP(t *testing.T) {
	body, err := json.Marshal(applicationList)
	require.NoError(t, err)
	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testProxyRequestPath, r.URL.Path)
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", "http://"+rpURL+testProxyRequestPath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", rpURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	ctrl := gomock.NewController(t)
	db := store.NewMockStorageClient(ctrl)

	router := mux.NewRouter()
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err = api.Register(ctx, router, controller.Options{
		DB:       db,
		BasePath: basePath,
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	// Register RP with UCP
	registerRP(t, ucp, ucpClient, db, true)

	// Create a Resource group
	createResourceGroup(t, ucp, ucpClient, db)

	// Send a request that will be proxied to the RP
	sendProxyRequest(t, ucp, ucpClient, db)
}

func Test_ProxyToRP_NonNativePlane(t *testing.T) {
	body, err := json.Marshal(applicationList)
	require.NoError(t, err)
	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testProxyRequestAzurePath, r.URL.Path)
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

	ctrl := gomock.NewController(t)
	db := store.NewMockStorageClient(ctrl)

	router := mux.NewRouter()
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err = api.Register(ctx, router, controller.Options{
		DB:       db,
		BasePath: basePath,
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	// Register RP with UCP
	registerRP(t, ucp, ucpClient, db, false)

	// Create a Resource group
	createResourceGroup(t, ucp, ucpClient, db)

	// Send a request that will be proxied to the RP
	sendProxyRequest_AzurePlane(t, ucp, ucpClient, db)
}

func Test_ProxyToRP_ResourceGroupDoesNotExist(t *testing.T) {
	ucp, ucpClient, db := initialize(t)
	// Send a request that will be proxied to the RP
	sendProxyRequest_ResourceGroupDoesNotExist(t, ucp, ucpClient, db)
}

func Test_MethodNotAllowed(t *testing.T) {
	ucp, ucpClient, _ := initialize(t)
	// Send a request that will be proxied to the RP
	request, err := http.NewRequest("DELETE", ucp.URL+basePath+"/planes", nil)
	require.NoError(t, err)
	response, err := ucpClient.httpClient.Do(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
}

func Test_NotFound(t *testing.T) {
	ucp, ucpClient, _ := initialize(t)
	// Send a request that will be proxied to the RP
	request, err := http.NewRequest("GET", ucp.URL+basePath+"/abc", nil)
	require.NoError(t, err)
	response, err := ucpClient.httpClient.Do(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func initialize(t *testing.T) (*httptest.Server, Client, *store.MockStorageClient) {
	body, err := json.Marshal(applicationList)
	require.NoError(t, err)
	rp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Location", "http://"+rpURL+testProxyRequestPath)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}))
	listener, err := net.Listen("tcp", rpURL)
	require.NoError(t, err)
	rp.Listener = listener
	defer listener.Close()

	rp.Start()
	defer rp.Close()

	ctrl := gomock.NewController(t)
	db := store.NewMockStorageClient(ctrl)

	router := mux.NewRouter()
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err = api.Register(ctx, router, controller.Options{
		DB:       db,
		BasePath: basePath,
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	// Register RP with UCP
	registerRP(t, ucp, ucpClient, db, true)

	return ucp, ucpClient, db
}

func registerRP(t *testing.T, ucp *httptest.Server, ucpClient Client, db *store.MockStorageClient, ucpNative bool) {
	var requestBody map[string]interface{}
	if ucpNative {
		requestBody = map[string]interface{}{
			"properties": map[string]interface{}{
				"resourceProviders": map[string]string{
					"Applications.Core": "http://" + rpURL,
				},
				"kind": rest.PlaneKindUCPNative,
			},
		}
	} else {
		requestBody = map[string]interface{}{
			"properties": map[string]interface{}{
				"kind": rest.PlaneKindAzure,
				"url":  "http://" + azureURL,
			},
		}
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	var createPlaneRequest *http.Request
	if ucpNative {
		createPlaneRequest, err = http.NewRequest("PUT", ucp.URL+basePath+"/planes/radius/local", bytes.NewBuffer(body))
	} else {
		createPlaneRequest, err = http.NewRequest("PUT", ucp.URL+basePath+"/planes/azure/azurecloud", bytes.NewBuffer(body))
	}
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any())
	db.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any())

	response, err := ucpClient.httpClient.Do(createPlaneRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	registerPlaneResponseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var responsePlane v20220901privatepreview.PlaneResource
	err = json.Unmarshal(registerPlaneResponseBody, &responsePlane)
	require.NoError(t, err)
	if ucpNative {
		assert.Equal(t, testUCPNativePlane, responsePlane)
	} else {
		assert.Equal(t, testAzurePlane, responsePlane)
	}
}

func createResourceGroup(t *testing.T, ucp *httptest.Server, ucpClient Client, db *store.MockStorageClient) {
	requestBody := map[string]interface{}{
		"name": "rg1",
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	db.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any())
	createResourceGroupRequest, err := http.NewRequest("PUT", ucp.URL+basePath+"/planes/radius/local/resourceGroups/rg1", bytes.NewBuffer(body))
	require.NoError(t, err)
	createResourceGroupResponse, err := ucpClient.httpClient.Do(createResourceGroupRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, createResourceGroupResponse.StatusCode)
	createResourceGroupResponseBody, err := io.ReadAll(createResourceGroupResponse.Body)
	require.NoError(t, err)
	var responseResourceGroup v20220901privatepreview.ResourceGroupResource
	err = json.Unmarshal(createResourceGroupResponseBody, &responseResourceGroup)
	require.NoError(t, err)
	assert.Equal(t, testResourceGroup, responseResourceGroup)
}

func sendProxyRequest(t *testing.T, ucp *httptest.Server, ucpClient Client, db *store.MockStorageClient) {
	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		data := store.Object{
			Metadata: store.Metadata{},
			Data:     testUCPNativePlane,
		}
		return &data, nil
	})

	rgID, err := resources.Parse("/planes/radius/local/resourceGroups/rg1")
	require.NoError(t, err)
	db.EXPECT().Get(gomock.Any(), rgID.String())

	proxyRequest, err := http.NewRequest("GET", ucp.URL+basePath+testProxyRequestPath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucpClient.httpClient.Do(proxyRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, proxyRequestResponse.StatusCode)
	assert.Equal(t, apiVersionQueyParam, proxyRequestResponse.Request.URL.RawQuery)
	assert.Equal(t, "http://"+proxyRequest.Host+basePath+testProxyRequestPath, proxyRequestResponse.Header["Location"][0])

	proxyRequestResponseBody, err := io.ReadAll(proxyRequestResponse.Body)
	require.NoError(t, err)
	responseAppList := []map[string]interface{}{}
	err = json.Unmarshal(proxyRequestResponseBody, &responseAppList)
	require.NoError(t, err)
	assert.Equal(t, applicationList, responseAppList)
}

func sendProxyRequest_AzurePlane(t *testing.T, ucp *httptest.Server, ucpClient Client, db *store.MockStorageClient) {
	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		data := store.Object{
			Metadata: store.Metadata{},
			Data:     testAzurePlane,
		}
		return &data, nil
	})

	proxyRequest, err := http.NewRequest("GET", ucp.URL+basePath+"/planes/azure/azurecloud"+testProxyRequestAzurePath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucpClient.httpClient.Do(proxyRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, proxyRequestResponse.StatusCode)
	assert.Equal(t, apiVersionQueyParam, proxyRequestResponse.Request.URL.RawQuery)

	proxyRequestResponseBody, err := io.ReadAll(proxyRequestResponse.Body)
	require.NoError(t, err)
	responseAppList := []map[string]interface{}{}
	err = json.Unmarshal(proxyRequestResponseBody, &responseAppList)
	require.NoError(t, err)
	assert.Equal(t, applicationList, responseAppList)
}

func sendProxyRequest_ResourceGroupDoesNotExist(t *testing.T, ucp *httptest.Server, ucpClient Client, db *store.MockStorageClient) {
	db.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		data := store.Object{
			Metadata: store.Metadata{},
			Data:     testUCPNativePlane,
		}
		return &data, nil
	})

	rgID, err := resources.Parse("/planes/radius/local/resourceGroups/rg1")
	require.NoError(t, err)

	db.EXPECT().Get(gomock.Any(), rgID.String()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	proxyRequest, err := http.NewRequest("GET", ucp.URL+basePath+testProxyRequestPath+"?"+apiVersionQueyParam, nil)
	require.NoError(t, err)
	proxyRequestResponse, err := ucpClient.httpClient.Do(proxyRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, proxyRequestResponse.StatusCode)
}
