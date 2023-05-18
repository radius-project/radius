// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestSqlDatabase_ConvertVersionedToDataModel(t *testing.T) {
	testsFile := "sqldatabaseresource.json"
	rawPayload := loadTestData(testsFile)
	var testset []TestData
	err := json.Unmarshal(rawPayload, &testset)
	require.NoError(t, err)
	for _, testData := range testset {
		versionedResource := &SQLDatabaseResource{}
		err := json.Unmarshal(testData.Payload, versionedResource)
		require.NoError(t, err)
		// act
		dm, err := versionedResource.ConvertTo()
		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.SqlDatabase)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0", convertedResource.ID)
		require.Equal(t, "sql0", convertedResource.Name)
		require.Equal(t, linkrp.SqlDatabasesResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if convertedResource.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
			if convertedResource.Properties.Resources != nil {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resources[0].ID)
			}
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", convertedResource.Properties.Server)
			require.Equal(t, "testDatabase", convertedResource.Properties.Database)
		} else {
			require.Equal(t, "sql-test", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
		}
	}
}

func TestSqlDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testFile := "sqldatabaseresourcedatamodel.json"
	rawPayload := loadTestData(testFile)
	var testset []TestData
	err := json.Unmarshal(rawPayload, &testset)
	require.NoError(t, err)
	for _, testData := range testset {
		resource := &datamodel.SqlDatabase{}
		err := json.Unmarshal(testData.Payload, resource)
		require.NoError(t, err)
		// act
		versionedResource := &SQLDatabaseResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0", *versionedResource.ID)
		require.Equal(t, "sql0", *versionedResource.Name)
		require.Equal(t, linkrp.SqlDatabasesResourceType, resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		v := versionedResource.Properties
		if *v.ResourceProvisioning == ResourceProvisioningManual {
			if v.Resources != nil {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", *v.Resources[0].ID)
			}
			require.Equal(t, "testAccount1.sql.cosmos.azure.com", *v.Server)
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
			require.Equal(t, "testDatabase", *v.Database)
		} else {
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
		description := testData.Description
		_, err = versionedResource.ConvertTo()
		if description == "invalid_resource_provisioning" {
			expectedErr := v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
			require.Equal(t, &expectedErr, err)
		}
		if description == "invalid_properties_for_manual_provisioning" {
			expectedErr := v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("database and server are required when resourceProvisioning is %s", ResourceProvisioningManual)}
			require.Equal(t, &expectedErr, err)
		}
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
