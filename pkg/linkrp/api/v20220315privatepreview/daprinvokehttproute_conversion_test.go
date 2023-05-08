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

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

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
		require.Equal(t, linkrp.DaprInvokeHttpRoutesResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "daprAppId", string(convertedResource.Properties.AppId))
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)

		if payload == "daprinvokehttprouteresource.json" {
			require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}

		if payload == "daprinvokehttprouteresource_recipe.json" {
			require.Equal(t, "dapr-test", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
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
		require.Equal(t, linkrp.DaprInvokeHttpRoutesResourceType, *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "daprAppId", string(*versionedResource.Properties.AppID))

		if payload == "daprinvokehttprouteresource.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "ExtenderProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}

		if payload == "daprinvokehttprouteresourcedatamodel_recipe.json" {
			require.Equal(t, "dapr-test", *versionedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", versionedResource.Properties.Recipe.Parameters["foo"])
		}
	}
}

func TestDaprInvokeHttpRoute_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprInvokeHTTPRouteResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
