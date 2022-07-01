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
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestExtender_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"extenderresource.json", "extenderresource2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &ExtenderResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		secrets := map[string]interface{}{"accountSid": "sid", "authToken:": "token"}
		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.Extender)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/extenders/extender0", convertedResource.ID)
		require.Equal(t, "extender0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/extenders", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "222-222-2222", convertedResource.Properties.AdditionalProperties["fromNumber"])
		if payload == "extenderresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			require.Equal(t, secrets, convertedResource.Properties.Secrets)
		}
	}
}

func TestExtender_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"extenderresourcedatamodel.json", "extenderresourcedatamodel2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.Extender{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &ExtenderResource{}
		err = versionedResource.ConvertFrom(resource)

		secrets := map[string]interface{}{"accountSid": "sid", "authToken:": "token"}
		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/extenders/extender0", resource.ID)
		require.Equal(t, "extender0", resource.Name)
		require.Equal(t, "Applications.Connector/extenders", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		require.Equal(t, "222-222-2222", resource.Properties.AdditionalProperties["fromNumber"])
		if payload == "extenderresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "ExtenderProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
			require.Equal(t, secrets, resource.Properties.Secrets)
		}
	}
}

func TestExtenderResponse_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"extenderresource.json", "extenderresource2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &ExtenderResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.Extender)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/extenders/extender0", convertedResource.ID)
		require.Equal(t, "extender0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/extenders", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "222-222-2222", convertedResource.Properties.AdditionalProperties["fromNumber"])
		if payload == "extenderresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestExtenderResponse_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"extenderresourcedatamodel.json", "extenderresourcedatamodel2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.Extender{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &ExtenderResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/extenders/extender0", resource.ID)
		require.Equal(t, "extender0", resource.Name)
		require.Equal(t, "Applications.Connector/extenders", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		require.Equal(t, "222-222-2222", resource.Properties.AdditionalProperties["fromNumber"])
		if payload == "extenderresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "ExtenderProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestExtender_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ExtenderResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
