// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestMongoDatabase_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabaseresource.json")
	versionedResource := &MongoDatabaseResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.MongoDatabase)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", convertedResource.ID)
	require.Equal(t, "mongo0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/mongoDatabases", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", convertedResource.Properties.Resource)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
}

func TestMongoDatabaseWithValues_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabaseresourcewithvalues.json")
	versionedResource := &MongoDatabaseResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.MongoDatabase)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", convertedResource.ID)
	require.Equal(t, "mongo0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/mongoDatabases", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "testAccount1.mongo.cosmos.azure.com", convertedResource.Properties.Host)
	require.Equal(t, 10255, convertedResource.Properties.Port)
	require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
	require.Equal(t, "testUser", convertedResource.Properties.Secrets.Username)
	require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)

}

func TestMongoDatabase_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabaseresourcedatamodel.json")
	resource := &datamodel.MongoDatabase{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &MongoDatabaseResource{}
	err = versionedResource.ConvertFrom(resource)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", resource.ID)
	require.Equal(t, "mongo0", resource.Name)
	require.Equal(t, "Applications.Connector/mongoDatabases", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", resource.Properties.Resource)
}

func TestMongoDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &MongoDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
