// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourceGroups

import (
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
)

const (
	radiusPlaneResourceURL     = "/planes/radius/local?api-version=2023-04-15-preview"
	radiusPlaneRequestFixture  = "../planes/testdata/radiusplane_v20230415preview_requestbody.json"
	radiusPlaneResponseFixture = "../planes/testdata/radiusplane_v20230415preview_responsebody.json"

	resourceGroupCollectionURL          = "/planes/radius/local/resourceGroups?api-version=2023-04-15-preview"
	resourceGroupResourceURL            = "/planes/radius/local/resourcegroups/test-rg?api-version=2023-04-15-preview"
	resourceGroupRequestFixture         = "testdata/resourcegroup_v20230415preview_requestbody.json"
	resourceGroupResponseFixture        = "testdata/resourcegroup_v20230415preview_responsebody.json"
	resourceGroupListResponseFixture    = "testdata/resourcegroup_v20230415preview_list_responsebody.json"
	resourceGroupUpdatedRequestFixture  = "testdata/resourcegroup_updated_v20230415preview_requestbody.json"
	resourceGroupUpdatedResponseFixture = "testdata/resourcegroup_updated_v20230415preview_responsebody.json"
)

func createRadiusPlane(server *testserver.TestServer) {
	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.EqualsFixture(200, radiusPlaneResponseFixture)
}

func Test_ResourceGroup_PUT_Create(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	response := server.MakeFixtureRequest("PUT", resourceGroupResourceURL, resourceGroupRequestFixture)
	response.EqualsFixture(200, resourceGroupResponseFixture)
}

func Test_ResourceGroup_PUT_Update(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	response := server.MakeFixtureRequest("PUT", resourceGroupResourceURL, resourceGroupRequestFixture)
	response.EqualsFixture(200, resourceGroupResponseFixture)

	response = server.MakeFixtureRequest("PUT", resourceGroupResourceURL, resourceGroupUpdatedRequestFixture)
	response.EqualsFixture(200, resourceGroupUpdatedResponseFixture)
}

func Test_ResourceGroup_GET_Empty(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	response := server.MakeRequest("GET", resourceGroupResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func Test_ResourceGroup_GET_Found(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	response := server.MakeFixtureRequest("PUT", resourceGroupResourceURL, resourceGroupRequestFixture)
	response.EqualsFixture(200, resourceGroupResponseFixture)

	response = server.MakeRequest("GET", resourceGroupResourceURL, nil)
	response.EqualsFixture(200, resourceGroupResponseFixture)
}

func Test_ResourceGroup_LIST(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	response := server.MakeFixtureRequest("PUT", resourceGroupResourceURL, resourceGroupRequestFixture)
	response.EqualsFixture(200, resourceGroupResponseFixture)

	response = server.MakeRequest("GET", resourceGroupCollectionURL, nil)
	response.EqualsFixture(200, resourceGroupListResponseFixture)
}
