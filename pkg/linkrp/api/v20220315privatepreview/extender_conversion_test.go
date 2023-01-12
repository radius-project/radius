// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
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

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.Extender)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/extenders/extender0", convertedResource.ID)
		require.Equal(t, "extender0", convertedResource.Name)
		require.Equal(t, "Applications.Link/extenders", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, map[string]any{"fromNumber": "222-222-2222"}, convertedResource.Properties.AdditionalProperties)

		if payload == "extenderresource.json" {
			require.Equal(t, map[string]any{"accountSid": "sid", "authToken:": "token"}, convertedResource.Properties.Secrets)
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		} else {
			require.Empty(t, convertedResource.Properties.Secrets)
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
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

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/extenders/extender0", *versionedResource.ID)
		require.Equal(t, "extender0", *versionedResource.Name)
		require.Equal(t, "Applications.Link/extenders", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, map[string]any{"fromNumber": "222-222-2222"}, versionedResource.Properties.AdditionalProperties)
		require.Empty(t, versionedResource.Properties.Secrets) // Secrets are omitted from the versioned data model.

		if payload == "extenderresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "ExtenderProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestExtender_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ExtenderResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
