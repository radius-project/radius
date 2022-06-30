// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/stretchr/testify/require"
)

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func loadTestData(testfile string) []byte {
	d, err := ioutil.ReadFile("./testdata/" + testfile)
	if err != nil {
		return nil
	}
	return d
}

func TestMongoDatabase_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"mongodatabaseresource2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", convertedResource.ID)
		require.Equal(t, "mongo0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/mongoDatabases", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", convertedResource.Properties.Resource)
		require.Equal(t, "testAccount1.mongo.cosmos.azure.com", convertedResource.Properties.Host)
		require.Equal(t, int32(10255), convertedResource.Properties.Port)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
	}
}

func TestMongoDatabaseResponse_ConvertVersionedToDataModel(t *testing.T) {
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
	require.Equal(t, "", convertedResource.Properties.Resource)
	require.Equal(t, "testAccount1.mongo.cosmos.azure.com", convertedResource.Properties.Host)
	require.Equal(t, int32(10255), convertedResource.Properties.Port)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
	require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
	require.Equal(t, "testUser", convertedResource.Properties.Secrets.Username)
	require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
}

func TestMongoDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"mongodatabaseresourcedatamodel.json", "mongodatabaseresourcedatamodel2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", *versionedResource.ID)
		require.Equal(t, "mongo0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/mongoDatabases", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *versionedResource.Properties.Host)
		require.Equal(t, int32(10255), *versionedResource.Properties.Port)
		if payload == "mongodatabaseresourcedatamodel.json" {
			require.Equal(t, "test-connection-string", *versionedResource.Properties.Secrets.ConnectionString)
			require.Equal(t, "testUser", *versionedResource.Properties.Secrets.Username)
			require.Equal(t, "testPassword", *versionedResource.Properties.Secrets.Password)
			require.Equal(t, "AzureCosmosAccount", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		} else {
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", *versionedResource.Properties.Resource)
		}
	}
}

func TestMongoDatabaseResponse_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("mongodatabaseresponseresourcedatamodel.json")
	resource := &datamodel.MongoDatabase{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &MongoDatabaseResource{}
	err = versionedResource.ConvertFrom(resource)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/mongoDatabases/mongo0", *versionedResource.ID)
	require.Equal(t, "mongo0", *versionedResource.Name)
	require.Equal(t, "Applications.Connector/mongoDatabases", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db", *versionedResource.Properties.Resource)
	require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *versionedResource.Properties.Host)
	require.Equal(t, int32(10255), *versionedResource.Properties.Port)
	require.Equal(t, "AzureCosmosAccount", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
}

func TestMongoDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
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
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
