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

package resourceproviders

import "github.com/radius-project/radius/pkg/ucp/testhost"

const (
	radiusAPIVersion           = "?api-version=2023-10-01-preview"
	radiusPlaneResourceURL     = "/planes/radius/local" + radiusAPIVersion
	radiusPlaneRequestFixture  = "../planes/testdata/radiusplane_v20231001preview_requestbody.json"
	radiusPlaneResponseFixture = "../planes/testdata/radiusplane_v20231001preview_responsebody.json"

	resourceProviderNamespace       = "Applications.Test"
	resourceProviderID              = "/planes/radius/local/providers/System.Resources/resourceproviders/" + resourceProviderNamespace
	resourceProviderCollectionURL   = "/planes/radius/local/providers/System.Resources/resourceproviders" + radiusAPIVersion
	resourceProviderURL             = resourceProviderID + radiusAPIVersion
	resourceProviderRequestFixture  = "testdata/resourceprovider_v20231001preview_requestbody.json"
	resourceProviderResponseFixture = "testdata/resourceprovider_v20231001preview_responsebody.json"

	resourceTypeName            = "testResources"
	resourceTypeID              = resourceProviderID + "/resourcetypes/" + resourceTypeName
	resourceTypeCollectionURL   = resourceProviderID + "/resourcetypes" + radiusAPIVersion
	resourceTypeURL             = resourceTypeID + radiusAPIVersion
	resourceTypeRequestFixture  = "testdata/resourcetype_v20231001preview_requestbody.json"
	resourceTypeResponseFixture = "testdata/resourcetype_v20231001preview_responsebody.json"

	apiVersionName            = "2025-01-01"
	apiVersionID              = resourceTypeID + "/apiversions/" + apiVersionName
	apiVersionCollectionURL   = resourceTypeID + "/apiversions" + radiusAPIVersion
	apiVersionURL             = apiVersionID + radiusAPIVersion
	apiVersionRequestFixture  = "testdata/apiversion_v20231001preview_requestbody.json"
	apiVersionResponseFixture = "testdata/apiversion_v20231001preview_responsebody.json"

	locationName            = "east"
	locationID              = resourceProviderID + "/locations/" + locationName
	locationCollectionURL   = resourceProviderID + "/locations" + radiusAPIVersion
	locationURL             = locationID + radiusAPIVersion
	locationRequestFixture  = "testdata/location_v20231001preview_requestbody.json"
	locationResponseFixture = "testdata/location_v20231001preview_responsebody.json"

	resourceProviderSummaryCollectionURL = "/planes/radius/local/providers" + radiusAPIVersion
	resourceProviderSummaryURL           = "/planes/radius/local/providers/" + resourceProviderNamespace + radiusAPIVersion

	manifestNamespace                     = "TestProvider.TestCompany"
	manifestResourceProviderID            = "/planes/radius/local/providers/System.Resources/resourceproviders/" + manifestNamespace
	manifestResourceProviderCollectionURL = "/planes/radius/local/providers/System.Resources/resourceproviders" + radiusAPIVersion
	manifestResourceProviderURL           = manifestResourceProviderID + radiusAPIVersion

	manifestResourceTypeName1           = "testResourcesAbc"
	manifestResourceTypeID              = manifestResourceProviderID + "/resourcetypes/" + manifestResourceTypeName1
	manifestResourceTypeCollectionURL   = manifestResourceProviderID + "/resourcetypes" + radiusAPIVersion
	manifestResourceTypeURL             = manifestResourceTypeID + radiusAPIVersion
	manifestResourceTypeRequestFixture  = "testdata/resourcetype_manifest_requestbody.json"
	manifestResourceTypeResponseFixture = "testdata/resourcetype_manifest_responsebody.json"
)

func createRadiusPlane(server *testhost.TestHost) {
	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", radiusPlaneResourceURL, nil)
	response.EqualsFixture(200, radiusPlaneResponseFixture)
}

func createResourceProvider(server *testhost.TestHost) {
	response := server.MakeFixtureRequest("PUT", resourceProviderURL, resourceProviderRequestFixture)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", resourceProviderURL, nil)
	response.EqualsFixture(200, resourceProviderResponseFixture)
}

func deleteResourceProvider(server *testhost.TestHost) {
	response := server.MakeRequest("DELETE", resourceProviderURL, nil)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", resourceProviderURL, nil)
	response.EqualsStatusCode(404)
}

func deleteManifestResourceProvider(server *testhost.TestHost) {
	response := server.MakeRequest("DELETE", manifestResourceProviderURL, nil)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", manifestResourceProviderURL, nil)
	response.EqualsStatusCode(404)
}

func createResourceType(server *testhost.TestHost) {
	response := server.MakeFixtureRequest("PUT", resourceTypeURL, resourceTypeRequestFixture)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", resourceTypeURL, nil)
	response.EqualsFixture(200, resourceTypeResponseFixture)
}

func deleteResourceType(server *testhost.TestHost) {
	response := server.MakeRequest("DELETE", resourceTypeURL, nil)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", resourceTypeURL, nil)
	response.EqualsStatusCode(404)
}

func createAPIVersion(server *testhost.TestHost) {
	response := server.MakeFixtureRequest("PUT", apiVersionURL, apiVersionRequestFixture)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", apiVersionURL, nil)
	response.EqualsFixture(200, apiVersionResponseFixture)
}

func deleteAPIVersion(server *testhost.TestHost) {
	response := server.MakeRequest("DELETE", apiVersionURL, nil)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", apiVersionURL, nil)
	response.EqualsStatusCode(404)
}

func createLocation(server *testhost.TestHost) {
	response := server.MakeFixtureRequest("PUT", locationURL, locationRequestFixture)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", locationURL, nil)
	response.EqualsFixture(200, locationResponseFixture)
}

func deleteLocation(server *testhost.TestHost) {
	response := server.MakeRequest("DELETE", locationURL, nil)
	response.WaitForOperationComplete(nil)

	response = server.MakeRequest("GET", locationURL, nil)
	response.EqualsStatusCode(404)
}
