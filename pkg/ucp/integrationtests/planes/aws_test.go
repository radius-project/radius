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
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
)

const (
	awsPlaneCollectionURL          = "/planes/aws?api-version=2023-10-01-preview"
	awsPlaneResourceURL            = "/planes/aws/aws?api-version=2023-10-01-preview"
	awsPlaneRequestFixture         = "testdata/awsplane_v20231001preview_requestbody.json"
	awsPlaneResponseFixture        = "testdata/awsplane_v20231001preview_responsebody.json"
	awsPlaneListResponseFixture    = "testdata/awsplane_v20231001preview_list_responsebody.json"
	awsPlaneUpdatedRequestFixture  = "testdata/awsplane_updated_v20231001preview_requestbody.json"
	awsPlaneUpdatedResponseFixture = "testdata/awsplane_updated_v20231001preview_responsebody.json"
)

func Test_AWSPlane_PUT_Create(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneRequestFixture)
	response.EqualsFixture(200, awsPlaneResponseFixture)
}

func Test_AWSPlane_PUT_Update(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneRequestFixture)
	response.EqualsFixture(200, awsPlaneResponseFixture)

	response = server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneUpdatedRequestFixture)
	response.EqualsFixture(200, awsPlaneUpdatedResponseFixture)
}

func Test_AWSPlane_GET_Empty(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeRequest("GET", awsPlaneResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func Test_AWSPlane_GET_Found(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneRequestFixture)
	response.EqualsFixture(200, awsPlaneResponseFixture)

	response = server.MakeRequest("GET", awsPlaneResourceURL, nil)
	response.EqualsFixture(200, awsPlaneResponseFixture)
}

func Test_AWSPlane_LIST(t *testing.T) {

	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneRequestFixture)
	response.EqualsFixture(200, awsPlaneResponseFixture)

	response = server.MakeRequest("GET", awsPlaneCollectionURL, nil)
	response.EqualsFixture(200, awsPlaneListResponseFixture)
}

func Test_AWSPlane_DELETE_DoesNotExist(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeRequest("DELETE", awsPlaneResourceURL, nil)
	response.EqualsResponse(204, nil)
}

func Test_AWSPlane_DELETE_Found(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", awsPlaneResourceURL, awsPlaneRequestFixture)
	response.EqualsFixture(200, awsPlaneResponseFixture)

	response = server.MakeRequest("DELETE", awsPlaneResourceURL, nil)
	response.EqualsResponse(200, nil)
}
