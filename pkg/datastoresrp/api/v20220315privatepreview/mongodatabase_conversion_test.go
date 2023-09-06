/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestMongoDatabase_ConvertVersionedToDataModel(t *testing.T) {
	testset := []struct {
		file     string
		desc     string
		expected *datamodel.MongoDatabase
	}{
		{
			// Opt-out with resources
			file: "mongodatabaseresource2.json",
			desc: "mongodb resource provisioning manual (with resources)",
			expected: &datamodel.MongoDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0",
						Name: "mongo0",
						Type: portableresources.MongoDatabasesResourceType,
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.MongoDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningManual,
					Host:                 "testAccount.mongo.cosmos.azure.com",
					Port:                 10255,
					Database:             "test-database",
					Resources:            []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db"}},
				},
			},
		},
		{
			desc: "mongodb resource named recipe",
			file: "mongodatabaseresource_recipe.json",
			expected: &datamodel.MongoDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0",
						Name: "mongo0",
						Type: portableresources.MongoDatabasesResourceType,
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.MongoDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningRecipe,
					Host:                 "testAccount.mongo.cosmos.azure.com",
					Port:                 10255,
					Recipe:               portableresources.LinkRecipe{Name: "cosmosdb", Parameters: map[string]interface{}{"foo": "bar"}},
				},
			},
		},

		{
			desc: "mongodb resource default recipe with overriden values",
			file: "mongodatabaseresource_recipe2.json",
			expected: &datamodel.MongoDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0",
						Name: "mongo0",
						Type: portableresources.MongoDatabasesResourceType,
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.MongoDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningRecipe,
					Host:                 "mynewhost.com",
					Port:                 10256,
					Recipe:               portableresources.LinkRecipe{Name: portableresources.DefaultRecipeName, Parameters: nil},
				},
			},
		},
		{
			desc: "mongodb resource provisioning manual (without resources)",
			file: "mongodatabaseresource.json",
			expected: &datamodel.MongoDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0",
						Name: "mongo0",
						Type: portableresources.MongoDatabasesResourceType,
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.MongoDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningManual,
					Host:                 "testAccount.mongo.cosmos.azure.com",
					Port:                 10255,
					Database:             "test-database",
					Username:             "testUser",
					Secrets: datamodel.MongoDatabaseSecrets{
						Password:         "testPassword",
						ConnectionString: "test-connection-string",
					},
				},
			},
		},
	}
	for _, tc := range testset {
		// arrange
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			versionedResource := &MongoDatabaseResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.MongoDatabase)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestMongoDatabase_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			payload: "mongodatabaseresource-invalidresprovisioning.json",
			errType: &v1.ErrModelConversion{},
			message: "$.properties.resourceProvisioning must be one of [manual recipe].",
		},
		{
			payload: "mongodatabaseresource-missinginputs.json",
			errType: &v1.ErrClientRP{},
			message: "code BadRequest: err multiple errors were found:\n\thost must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual\n\tdatabase must be specified when resourceProvisioning is set to manual",
		},
	}
	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(test.payload)
			versionedResource := &MongoDatabaseResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()
			require.Error(t, err)
			require.Nil(t, dm)
			require.IsType(t, test.errType, err)
			require.Equal(t, test.message, err.Error())
		})
	}
}

func TestMongoDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testset := []struct {
		file     string
		desc     string
		expected *MongoDatabaseResource
	}{
		{
			desc: "mongodb resource provisioning manual datamodel (without resources)",
			file: "mongodatabaseresourcedatamodel.json",
			expected: &MongoDatabaseResource{
				Location: to.Ptr(""),
				Properties: &MongoDatabaseProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Host:                 to.Ptr("testAccount1.mongo.cosmos.azure.com"),
					Port:                 to.Ptr(int32(10255)),
					Database:             to.Ptr("test-database"),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr(""), Parameters: nil},
					Username:             to.Ptr("testUser"),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0"),
				Name: to.Ptr("mongo0"),
				Type: to.Ptr(portableresources.MongoDatabasesResourceType),
			},
		},
		{
			desc: "mongodb resource provisioning manual datamodel (with resources)",
			file: "mongodatabaseresourcedatamodel2.json",
			expected: &MongoDatabaseResource{
				Location: to.Ptr(""),
				Properties: &MongoDatabaseProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Host:                 to.Ptr("testAccount1.mongo.cosmos.azure.com"),
					Port:                 to.Ptr(int32(10255)),
					Database:             to.Ptr("test-database"),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr(""), Parameters: nil},
					Resources:            []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.DocumentDB/databaseAccounts/testAccount/mongodbDatabases/db")}},
					Username:             to.Ptr(""),
					Status: &ResourceStatus{
						OutputResources: nil,
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0"),
				Name: to.Ptr("mongo0"),
				Type: to.Ptr(portableresources.MongoDatabasesResourceType),
			},
		},
		{
			// Named recipe
			desc: "mongodb named recipe datamodel",
			file: "mongodatabaseresourcedatamodel_recipe.json",
			expected: &MongoDatabaseResource{
				Location: to.Ptr(""),
				Properties: &MongoDatabaseProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					Host:                 to.Ptr("testAccount1.mongo.cosmos.azure.com"),
					Port:                 to.Ptr(int32(10255)),
					Database:             to.Ptr(""),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr("cosmosdb"), Parameters: map[string]interface{}{"foo": "bar"}},
					Username:             to.Ptr(""),
					Status: &ResourceStatus{
						OutputResources: nil,
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0"),
				Name: to.Ptr("mongo0"),
				Type: to.Ptr(portableresources.MongoDatabasesResourceType),
			},
		},
	}
	for _, tc := range testset {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			resource := &datamodel.MongoDatabase{}
			err := json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &MongoDatabaseResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestMongoDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
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
	rawPayload := testutil.ReadFixture("mongodatabasesecrets.json")
	versioned := &MongoDatabaseSecrets{}
	err := json.Unmarshal(rawPayload, versioned)
	require.NoError(t, err)

	// act
	dm, err := versioned.ConvertTo()

	// assert
	require.NoError(t, err)
	converted := dm.(*datamodel.MongoDatabaseSecrets)
	require.Equal(t, "test-connection-string", converted.ConnectionString)
	require.Equal(t, "testPassword", converted.Password)
}

func TestMongoDatabaseSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("mongodatabasesecretsdatamodel.json")
	secrets := &datamodel.MongoDatabaseSecrets{}
	err := json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &MongoDatabaseSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.ConnectionString)
	require.Equal(t, "testPassword", secrets.Password)
}

func TestMongoDatabaseSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
