// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestSqlDatabase_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("sqldatabaseresource.json")
	versionedResource := &SQLDatabaseResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.SqlDatabase)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/sqlDatabases/sql0", convertedResource.ID)
	require.Equal(t, "sql0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/sqlDatabases", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
	require.Equal(t, "testAccount1.sql.cosmos.azure.com", convertedResource.Properties.Server)
	require.Equal(t, "testDatabase", convertedResource.Properties.Database)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
}

func TestSqlDatabase_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("sqldatabaseresourcedatamodel.json")
	resource := &datamodel.SqlDatabase{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &SQLDatabaseResource{}
	err = versionedResource.ConvertFrom(resource)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/sqlDatabases/sql0", resource.ID)
	require.Equal(t, "sql0", resource.Name)
	require.Equal(t, "Applications.Connector/sqlDatabases", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", resource.Properties.Resource)
	require.Equal(t, "testAccount1.sql.cosmos.azure.com", resource.Properties.Server)
	require.Equal(t, "testDatabase", resource.Properties.Database)
}

func TestSqlDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SQLDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
