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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testrp"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

const (
	apiVersionParameter   = "api-version=2022-09-01-privatepreview"
	testRadiusPlaneID     = "/planes/radius/test"
	testResourceNamespace = "Applications.Test"
	testResourceGroupID   = testRadiusPlaneID + "/resourceGroups/test-rg"
	testResourceID        = testResourceGroupID + "/providers/Applications.Test/testResources/test-resource"
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
	require.Equal(t, "the resource with id '/planes/radius/test/resourceGroups/test-rg/providers/Applications.Test/testResources/test-resource' was not found", response.Error.Error.Message)
}

func Test_RadiusPlane_Proxy_Success(t *testing.T) {
	ucp := testserver.StartWithETCD(t, api.DefaultModules)
	rp := testrp.Start(t)

	data := map[string]string{
		"message": "here is some test data",
	}
	body, err := json.Marshal(data)
	require.NoError(t, err)

	rp.Handler = func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, strings.ToLower(testResourceID), strings.ToLower(r.URL.Path))
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("SomeHeader", "SomeValue")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
	}

	rps := map[string]*string{
		testResourceNamespace: to.Ptr("http://" + rp.Address()),
	}
	createRadiusPlane(ucp, rps)

	createResourceGroup(ucp, testResourceGroupID)

	response := ucp.MakeRequest(http.MethodGet, testResourceID, nil)
	response.EqualsResponse(http.StatusOK, body)
	require.Equal(t, "SomeValue", response.Raw.Header.Get("SomeHeader"))
}

func createRadiusPlane(ucp *testserver.TestServer, resourceProviders map[string]*string) {
	body := v20220901privatepreview.PlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20220901privatepreview.PlaneResourceProperties{
			Kind:              to.Ptr(v20220901privatepreview.PlaneKindUCPNative),
			ResourceProviders: resourceProviders,
		},
	}
	response := ucp.MakeTypedRequest(http.MethodPut, testRadiusPlaneID+"?"+apiVersionParameter, body)
	response.EqualsStatusCode(http.StatusOK)
}

func createResourceGroup(ucp *testserver.TestServer, id string) {
	body := v20220901privatepreview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20220901privatepreview.BasicResourceProperties{},
	}
	response := ucp.MakeTypedRequest(http.MethodPut, id+"?"+apiVersionParameter, body)
	response.EqualsStatusCode(http.StatusOK)
}
