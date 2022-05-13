// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestApplicationConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("applicationresource.json")
	r := &ApplicationResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	ct := dm.(*datamodel.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app0", ct.ID)
	require.Equal(t, "app0", ct.Name)
	require.Equal(t, "Applications.Core/applications", ct.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0", ct.Properties.Environment)
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)

}

func TestApplicationConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("applicationresourcedatamodel.json")
	r := &datamodel.Application{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &ApplicationResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app0", r.ID)
	require.Equal(t, "app0", r.Name)
	require.Equal(t, "Applications.Core/applications", r.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0", r.Properties.Environment)
}

func TestApplicationConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ApplicationResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
