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
	globalPlaneCollectionURL   = "/planes?api-version=2022-09-01-privatepreview"
	globalPlaneResponseFixture = "testdata/globalplane_v20220901privatepreview_list_responsebody.json"
)

func Test_AllPlanes_LIST(t *testing.T) {
	t.Skip("This functionality is currently broken. See https://github.com/project-radius/radius/issues/4877")

	server := testserver.Start(t)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", radiusPlaneResourceURL, radiusPlaneRequestFixture)
	response.EqualsFixture(200, radiusPlaneResponseFixture)

	response = server.MakeRequest("GET", globalPlaneCollectionURL, nil)
	response.EqualsFixture(200, radiusPlaneResponseFixture)
}
