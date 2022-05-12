// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestHTTPRouteConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("httprouteresource.json")
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
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)
}

func TestHTTPRouteConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("httprouteresourcedatamodel.json")
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
}

func TestHTTPRouteConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &HTTPRouteResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
