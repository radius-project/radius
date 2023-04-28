// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"os"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/stretchr/testify/require"
)

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func loadTestData(testfile string) []byte {
	d, err := os.ReadFile("./testdata/" + testfile)
	if err != nil {
		return nil
	}
	return d
}

func TestMongoDatabase_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"mongodatabaseresource2.json", "mongodatabaseresource_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &MongoDatabaseResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.MongoDatabase)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0", convertedResource.ID)
		require.Equal(t, "mongo0", convertedResource.Name)
		require.Equal(t, linkrp.N_MongoDatabasesResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "testAccount1.mongo.cosmos.azure.com", convertedResource.Properties.Host)
		require.Equal(t, int32(10255), convertedResource.Properties.Port)
		if payload == "mongodatabaseresource_recipe.json" {
			require.Equal(t, "cosmosdb", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
		}
	}
}

func TestMongoDatabase_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []string{"mongodatabaseresource_invalidmode.json", "mongodatabaseresource_invalidmode2.json", "mongodatabaseresource_invalidmode3.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &MongoDatabaseResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)
		var expectedErr v1.ErrClientRP
		if payload == "mongodatabaseresource_invalidmode.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "Unsupported mode abc"
		}
		if payload == "mongodatabaseresource_invalidmode2.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "resource is a required property for mode \"resource\""
		}
		if payload == "mongodatabaseresource_invalidmode3.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "recipe is a required property for mode \"recipe\""
		}
		if payload == "mongodatabaseresource_invalidmode4.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "rhost and port are required properties for mode \"values\""
		}
		_, err = versionedResource.ConvertTo()
		require.Equal(t, &expectedErr, err)
	}
}

func TestMongoDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"mongodatabaseresourcedatamodel.json", "mongodatabaseresourcedatamodel2.json", "mongodatabaseresourcedatamodel_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.MongoDatabase{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &MongoDatabaseResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0", *versionedResource.ID)
		require.Equal(t, "mongo0", *versionedResource.Name)
		require.Equal(t, linkrp.N_MongoDatabasesResourceType, *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetMongoDatabaseProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetMongoDatabaseProperties().Environment)
		switch v := versionedResource.Properties.(type) {
		case *ResourceMongoDatabaseProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", *v.Resource)
			require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
		case *RecipeMongoDatabaseProperties:
			require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
			require.Equal(t, "cosmosdb", *v.Recipe.Name)
			require.Equal(t, "bar", v.Recipe.Parameters["foo"])
		case *ValuesMongoDatabaseProperties:
			require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
			require.Equal(t, "AzureCosmosAccount", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", v.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestMongoDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &MongoDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestMongoDatabaseSecrets_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabasesecrets.json")
	versioned := &MongoDatabaseSecrets{}
	err := json.Unmarshal(rawPayload, versioned)
	require.NoError(t, err)

	// act
	dm, err := versioned.ConvertTo()

	// assert
	require.NoError(t, err)
	converted := dm.(*datamodel.MongoDatabaseSecrets)
	require.Equal(t, "test-connection-string", converted.ConnectionString)
	require.Equal(t, "testUser", converted.Username)
	require.Equal(t, "testPassword", converted.Password)
}

func TestMongoDatabaseSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabasesecretsdatamodel.json")
	secrets := &datamodel.MongoDatabaseSecrets{}
	err := json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &MongoDatabaseSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.ConnectionString)
	require.Equal(t, "testUser", secrets.Username)
	require.Equal(t, "testPassword", secrets.Password)
}

func TestMongoDatabaseSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
