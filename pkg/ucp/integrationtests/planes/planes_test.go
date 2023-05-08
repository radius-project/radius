/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package planes

import (
	"testing"

	"github.com/project-radius/radius/pkg/ucp/integrationtests/testserver"
)

const (
	globalPlaneCollectionURL   = "/planes?api-version=2022-09-01-privatepreview"
	planeTypeCollectionURL     = "/planes/radius?api-version=2022-09-01-privatepreview"
	globalPlaneResponseFixture = "testdata/globalplane_v20220901privatepreview_list_responsebody.json"
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
