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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestSqlDatabase_ConvertVersionedToDataModel(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *datamodel.SqlDatabase
	}{
		{
			desc: "sqldatabase manual resource",
			file: "sqldatabase_manual_resource.json",
			expected: &datamodel.SqlDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0",
						Name:     "sql0",
						Type:     linkrp.SqlDatabasesResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.SqlDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningManual,
					Resources: []*linkrp.ResourceReference{
						{
							ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase",
						},
					},
					Database: "testDatabase",
					Server:   "testAccount1.sql.cosmos.azure.com",
					Port:     1433,
					Username: "testUser",
					Secrets: datamodel.SqlDatabaseSecrets{
						Password:         "testPassword",
						ConnectionString: "test-connection-string",
					},
				},
			},
		},
		{
			desc: "sqldatabase recipe resource",
			file: "sqldatabase_recipe_resource.json",
			expected: &datamodel.SqlDatabase{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0",
						Name:     "sql0",
						Type:     linkrp.SqlDatabasesResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.SqlDatabaseProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningRecipe,
					Recipe: linkrp.LinkRecipe{
						Name: "sql-test",
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// arrange
			rawPayload, err := LoadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			versionedResource := &SQLDatabaseResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.SqlDatabase)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestSqlDatabase_ConvertDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *SQLDatabaseResource
	}{
		{
			desc: "sqldatabase manual resource datamodel",
			file: "sqldatabase_manual_resourcedatamodel.json",
			expected: &SQLDatabaseResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &SQLDatabaseProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Resources: []*ResourceReference{
						{
							ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Sql/servers/testServer/databases/testDatabase"),
						},
					},
					Database:          to.Ptr("testDatabase"),
					Server:            to.Ptr("testAccount1.sql.cosmos.azure.com"),
					Port:              to.Ptr(int32(1433)),
					Username:          to.Ptr("testUser"),
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "azure",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0"),
				Name: to.Ptr("sql0"),
				Type: to.Ptr(linkrp.SqlDatabasesResourceType),
			},
		},
		{
			desc: "sqldatabase recipe resource datamodel",
			file: "sqldatabase_recipe_resourcedatamodel.json",
			expected: &SQLDatabaseResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &SQLDatabaseProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					Database:             to.Ptr("testDatabase"),
					Port:                 to.Ptr(int32(1433)),
					Username:             to.Ptr("testUser"),
					Server:               to.Ptr("testAccount1.sql.cosmos.azure.com"),
					Recipe: &Recipe{
						Name: to.Ptr("sql-test"),
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "azure",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/sqlDatabases/sql0"),
				Name: to.Ptr("sql0"),
				Type: to.Ptr(linkrp.SqlDatabasesResourceType),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload, err := LoadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			resource := &datamodel.SqlDatabase{}
			err = json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &SQLDatabaseResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestSqlDatabase_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			"sqldatabase_invalid_properties_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err multiple errors were found:\n\tserver must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual\n\tdatabase must be specified when resourceProvisioning is set to manual",
		},
		{
			"sqldatabase_invalid_resourceprovisioning_resource.json",
			&v1.ErrModelConversion{},
			"$.properties.resourceProvisioning must be one of [manual recipe].",
		},
	}

	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload, err := LoadTestData("./testdata/" + test.payload)
			require.NoError(t, err)
			versionedResource := &SQLDatabaseResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()
			require.Error(t, err)
			require.Nil(t, dm)
			require.IsType(t, test.errType, err)
			require.Equal(t, test.message, err.Error())
		})
	}
}

func TestSqlDatabase_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SQLDatabaseResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestSqlDatabaseSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload, err := LoadTestData("./testdata/sqldatabase_secrets_datamodel.json")
	require.NoError(t, err)
	secrets := &datamodel.SqlDatabaseSecrets{}
	err = json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &SQLDatabaseSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.ConnectionString)
	require.Equal(t, "testPassword", secrets.Password)
}

func TestSqlDatabaseSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SQLDatabaseSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
