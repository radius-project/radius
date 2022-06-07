// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestDaprInvokeHttpRoute_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("daprinvokehttprouteresource.json")
	versionedResource := &DaprInvokeHTTPRouteResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	resourceType := map[string]interface{}{"Provider": "DaprInvokeHttpRouteProvider", "Type": "HttpRoute"}
	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.DaprInvokeHttpRoute)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprInvokeHttpRoutes/daprHttpRoute0", convertedResource.ID)
	require.Equal(t, "daprHttpRoute0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/daprInvokeHttpRoutes", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "daprAppId", string(convertedResource.Properties.AppId))
	require.Equal(t, "Deployment", convertedResource.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, convertedResource.Properties.Status.OutputResources[0]["ResourceType"])
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
}

func TestDaprInvokeHttpRoute_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("daprinvokehttprouteresourcedatamodel.json")
	resource := &datamodel.DaprInvokeHttpRoute{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &DaprInvokeHTTPRouteResource{}
	err = versionedResource.ConvertFrom(resource)

	resourceType := map[string]interface{}{"Provider": "DaprInvokeHttpRouteProvider", "Type": "HttpRoute"}

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprInvokeHttpRoutes/daprHttpRoute0", resource.ID)
	require.Equal(t, "daprHttpRoute0", resource.Name)
	require.Equal(t, "Applications.Connector/daprInvokeHttpRoutes", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "daprAppId", string(resource.Properties.AppId))
	require.Equal(t, "Deployment", resource.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, resource.Properties.Status.OutputResources[0]["ResourceType"])
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
