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

package azure

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testrp"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

const (
	testProxyRequestAzurePath = "/subscriptions/sid/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1"
	apiVersionParameter       = "api-version=2022-09-01-privatepreview"
	testAzurePlaneID          = "/planes/azure/test"
)

func Test_AzurePlane_ProxyRequest(t *testing.T) {
	ucp := testserver.StartWithETCD(t, api.DefaultModules)

	data := map[string]string{
		"message":         "here is some test data",
		"another message": "we don't transform the payloads we get from azure, so the shape of this data is not important",
	}
	body, err := json.Marshal(&data)
	require.NoError(t, err)

	rp := testrp.Start(t)
	rp.Handler = func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, strings.ToLower(testProxyRequestAzurePath), strings.ToLower(r.URL.Path))
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("SomeHeader", "SomeValue")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}

	createAzurePlane(ucp, rp)

	response := ucp.MakeRequest(http.MethodGet, testAzurePlaneID+testProxyRequestAzurePath, nil)
	response.EqualsResponse(http.StatusOK, body)
	require.Equal(t, "SomeValue", response.Raw.Header.Get("SomeHeader"))
}

func createAzurePlane(ucp *testserver.TestServer, rp *testrp.Server) {
	body := v20220901privatepreview.PlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20220901privatepreview.PlaneResourceProperties{
			Kind: to.Ptr(v20220901privatepreview.PlaneKindAzure),
			URL:  to.Ptr("http://" + rp.Address()),
		},
	}
	response := ucp.MakeTypedRequest(http.MethodPut, testAzurePlaneID+"?"+apiVersionParameter, body)
	response.EqualsStatusCode(http.StatusOK)
}
