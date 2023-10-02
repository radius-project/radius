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

package v20231001preview

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"

	"github.com/stretchr/testify/require"
)

func TestHTTPRouteConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("httprouteresource.json")
	r := &HTTPRouteResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	ct := dm.(*datamodel.HTTPRoute)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/route0", ct.ID)
	require.Equal(t, "route0", ct.Name)
	require.Equal(t, "Applications.Core/httpRoutes", ct.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", ct.Properties.Application)
	require.Equal(t, "localhost", ct.Properties.Hostname)
	require.Equal(t, int32(8080), ct.Properties.Port)
	require.Equal(t, "http", ct.Properties.Scheme)
	require.Equal(t, "http://testapplications.com/httproute/", ct.Properties.URL)
	require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, "2023-10-01-preview", ct.InternalMetadata.UpdatedAPIVersion)
}

func TestHTTPRouteConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("httprouteresourcedatamodel.json")
	r := &datamodel.HTTPRoute{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &HTTPRouteResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/route0", r.ID)
	require.Equal(t, "route0", r.Name)
	require.Equal(t, "Applications.Core/httpRoutes", r.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", r.Properties.Application)
	require.Equal(t, "localhost", r.Properties.Hostname)
	require.Equal(t, int32(8080), r.Properties.Port)
	require.Equal(t, "http", r.Properties.Scheme)
	require.Equal(t, "http://testapplications.com/httproute/", r.Properties.URL)
	require.Equal(t, resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}), versioned.Properties.Status)
}

func TestHTTPRouteConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &HTTPRouteResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
