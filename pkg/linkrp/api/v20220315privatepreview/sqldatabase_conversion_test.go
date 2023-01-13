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
	"github.com/stretchr/testify/require"
)

func TestSqlDatabase_ConvertVersionedToDataModel(t *testing.T) {

	testset := []string{"sqldatabaseresource.json", "sqldatabaseresource2.json", "sqldatabaseresource_recipe.json", "sqldatabaseresourcemodevalues.json"}
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

		switch versionedResource.Properties.(type) {
		case *ResourceSQLDatabaseProperties:
			require.Equal(t, "resource", string(convertedResource.Properties.Mode))
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
		case *ValuesSQLDatabaseProperties:
			require.Equal(t, "values", string(convertedResource.Properties.Mode))
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", convertedResource.Properties.Server)
			require.Equal(t, "testDatabase", convertedResource.Properties.Database)
		case *RecipeSQLDatabaseProperties:
			require.Equal(t, "recipe", string(convertedResource.Properties.Mode))
			require.Equal(t, "sql-test", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
		}
	}
}

func TestSqlDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"sqldatabaseresourcedatamodel.json", "sqldatabaseresourcedatamodel2.json", "sqldatabaseresourcedatamodel_recipe.json", "sqldatabaseresourcemodevaluesdatamodel.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0", *versionedResource.ID)
		require.Equal(t, "sql0", *versionedResource.Name)
		require.Equal(t, "Applications.Link/sqlDatabases", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetSQLDatabaseProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetSQLDatabaseProperties().Environment)
		switch v := versionedResource.Properties.(type) {
		case *ResourceSQLDatabaseProperties:
			require.Equal(t, "resource", string(*v.Mode))
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", *v.Resource)
		case *ValuesSQLDatabaseProperties:
			require.Equal(t, "values", string(*v.Mode))
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", *v.Server)
			require.Equal(t, "Deployment", versionedResource.Properties.GetSQLDatabaseProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.GetSQLDatabaseProperties().Status.OutputResources[0]["Provider"])
			require.Equal(t, "testDatabase", *v.Database)
		case *RecipeSQLDatabaseProperties:
			require.Equal(t, "recipe", string(*v.Mode))
			require.Equal(t, "sql-test", *v.Recipe.Name)
			require.Equal(t, "bar", v.Recipe.Parameters["foo"])
		}
	}
}

func TestSqlDatabase_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testsFile := "sqldatabaseinvalid.json"
	rawPayload := loadTestData(testsFile)
	var testset []TestData
	err := json.Unmarshal(rawPayload, &testset)
	require.NoError(t, err)
	for _, testData := range testset {
		versionedResource := &SQLDatabaseResource{}
		err := json.Unmarshal(testData.Payload, versionedResource)
		require.NoError(t, err)
		var expectedErr v1.ErrClientRP
		description := testData.Description
		if description == "unsupported_mode" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "Unsupported mode abc"
		}
		if description == "invalid_properties_with_mode_resource" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "resource is a required property for mode 'resource'"
		}
		if description == "invalid_properties_with_mode_recipe" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "recipe is a required property for mode 'recipe'"
		}
		if description == "invalid_properties_with_mode_values" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "database/server are required properties for mode 'values'"
		}
		_, err = versionedResource.ConvertTo()
		require.Equal(t, &expectedErr, err)
	}
}

func TestSqlDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SQLDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

type TestData struct {
	Description string          `json:"description,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}
