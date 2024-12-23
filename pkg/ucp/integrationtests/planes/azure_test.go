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

package planes

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/testhost"
)

const (
	azurePlaneCollectionURL          = "/planes/azure?api-version=2023-10-01-preview"
	azurePlaneResourceURL            = "/planes/azure/azurecloud?api-version=2023-10-01-preview"
	azurePlaneRequestFixture         = "testdata/azureplane_v20231001preview_requestbody.json"
	azurePlaneResponseFixture        = "testdata/azureplane_v20231001preview_responsebody.json"
	azurePlaneListResponseFixture    = "testdata/azureplane_v20231001preview_list_responsebody.json"
	azurePlaneUpdatedRequestFixture  = "testdata/azureplane_updated_v20231001preview_requestbody.json"
	azurePlaneUpdatedResponseFixture = "testdata/azureplane_updated_v20231001preview_responsebody.json"
)

func Test_AzurePlane_PUT_Create(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneRequestFixture)
	response.EqualsFixture(200, azurePlaneResponseFixture)
}

func Test_AzurePlane_PUT_Update(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneRequestFixture)
	response.EqualsFixture(200, azurePlaneResponseFixture)

	response = server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneUpdatedRequestFixture)
	response.EqualsFixture(200, azurePlaneUpdatedResponseFixture)
}

func Test_AzurePlane_GET_Empty(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeRequest("GET", azurePlaneResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func Test_AzurePlane_GET_Found(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneRequestFixture)
	response.EqualsFixture(200, azurePlaneResponseFixture)

	response = server.MakeRequest("GET", azurePlaneResourceURL, nil)
	response.EqualsFixture(200, azurePlaneResponseFixture)
}

func Test_AzurePlane_LIST(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	// Add a azure plane
	response := server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneRequestFixture)
	response.EqualsFixture(200, azurePlaneResponseFixture)

	// Verify that /planes/azure URL returns planes only with the azure plane type.
	response = server.MakeRequest("GET", azurePlaneCollectionURL, nil)
	response.EqualsFixture(200, azurePlaneListResponseFixture)
}

func Test_AzurePlane_DELETE_DoesNotExist(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeRequest("DELETE", azurePlaneResourceURL, nil)
	response.EqualsResponse(204, nil)
}

func Test_AzurePlane_DELETE_Found(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", azurePlaneResourceURL, azurePlaneRequestFixture)
	response.EqualsFixture(200, azurePlaneResponseFixture)

	response = server.MakeRequest("DELETE", azurePlaneResourceURL, nil)
	response.EqualsResponse(200, nil)
}
