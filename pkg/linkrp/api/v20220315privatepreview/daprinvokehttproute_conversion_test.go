// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestDaprInvokeHttpRoute_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"daprinvokehttprouteresource.json", "daprinvokehttprouteresource2.json", "daprinvokehttprouteresource_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &DaprInvokeHTTPRouteResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprInvokeHttpRoute)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprInvokeHttpRoutes/daprHttpRoute0", convertedResource.ID)
		require.Equal(t, "daprHttpRoute0", convertedResource.Name)
		require.Equal(t, "Applications.Link/daprInvokeHttpRoutes", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		switch versionedResource.Properties.(type) {
		case *ValuesDaprInvokeHTTPRouteProperties:
			require.Equal(t, "daprAppId", string(convertedResource.Properties.AppId))
			require.Equal(t, "values", string(convertedResource.Properties.Mode))
		case *RecipeDaprInvokeHTTPRouteProperties:
			require.Equal(t, "recipe", string(convertedResource.Properties.Mode))
			require.Equal(t, "dapr-test", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestDaprInvokeHttpRoute_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"daprinvokehttprouteresourcedatamodel.json", "daprinvokehttprouteresourcedatamodel2.json", "daprinvokehttprouteresourcedatamodel_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.DaprInvokeHttpRoute{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &DaprInvokeHTTPRouteResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprInvokeHttpRoutes/daprHttpRoute0", *versionedResource.ID)
		require.Equal(t, "daprHttpRoute0", *versionedResource.Name)
		require.Equal(t, "Applications.Link/daprInvokeHttpRoutes", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetDaprInvokeHTTPRouteProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetDaprInvokeHTTPRouteProperties().Environment)
		switch v := versionedResource.Properties.(type) {
		case *ValuesDaprInvokeHTTPRouteProperties:
			require.Equal(t, "values", string(*v.Mode))
			require.Equal(t, "daprAppId", string(*v.AppID))
		case *RecipeDaprInvokeHTTPRouteProperties:
			require.Equal(t, "recipe", string(*v.Mode))
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "DaprInvokeHttpRouteProvider", v.Status.OutputResources[0]["Provider"])
			require.Equal(t, "dapr-test", *v.Recipe.Name)
			require.Equal(t, "bar", v.Recipe.Parameters["foo"])
		}
	}
}

func TestDaprInvokeHttpRoute_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprInvokeHTTPRouteResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
