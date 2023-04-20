// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package planes

import (
	"testing"

	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
)

const (
	globalPlaneCollectionURL   = "/planes?api-version=2023-04-15-preview"
	planeTypeCollectionURL     = "/planes/radius?api-version=2023-04-15-preview"
	globalPlaneResponseFixture = "testdata/globalplane_v20230415preview_list_responsebody.json"
)

func Test_AllPlanes_LIST(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.EqualsFixture(200, radiusPlaneResponseFixture)

	response = server.MakeRequest("GET", globalPlaneCollectionURL, nil)
	response.EqualsFixture(200, radiusPlaneListResponseFixture)
}

func Test_AllPlanes_LIST_BY_TYPE(t *testing.T) {
	server := testserver.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.EqualsFixture(200, radiusPlaneResponseFixture)

	response = server.MakeRequest("GET", planeTypeCollectionURL, nil)
	response.EqualsFixture(200, radiusPlaneListResponseFixture)
}
