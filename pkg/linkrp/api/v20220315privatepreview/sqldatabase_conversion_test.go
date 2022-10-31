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

func TestSqlDatabase_ConvertVersionedToDataModel(t *testing.T) {

	testset := []string{"sqldatabaseresource.json", "sqldatabaseresource2.json", "sqldatabaseresource_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &SQLDatabaseResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.SqlDatabase)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0", convertedResource.ID)
		require.Equal(t, "sql0", convertedResource.Name)
		require.Equal(t, "Applications.Link/sqlDatabases", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)

		if payload == "sqldatabaseresource.json" {
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", convertedResource.Properties.Server)
			require.Equal(t, "testDatabase", convertedResource.Properties.Database)
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}

		if payload == "sqldatabaseresource_recipe.json" {
			require.Equal(t, "sql-test", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
		} else {
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
		}
	}
}

func TestSqlDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"sqldatabaseresourcedatamodel.json", "sqldatabaseresourcedatamodel2.json", "sqldatabaseresourcedatamodel_recipe.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.SqlDatabase{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &SQLDatabaseResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0", resource.ID)
		require.Equal(t, "sql0", resource.Name)
		require.Equal(t, "Applications.Link/sqlDatabases", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)

		if payload == "sqldatabaseresourcedatamodel.json" {
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", resource.Properties.Server)
			require.Equal(t, "testDatabase", resource.Properties.Database)
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}

		if payload == "sqldatabaseresourcedatamodel.json" || payload == "sqldatabaseresourcedatamodel2.json" {
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", resource.Properties.Resource)
		} else {
			require.Equal(t, "sql-test", *versionedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", versionedResource.Properties.Recipe.Parameters["foo"])
		}
	}
}

func TestSqlDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SQLDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
