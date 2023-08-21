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

package mongodatabases

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/linkrp"
	"github.com/radius-project/radius/pkg/linkrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp/processors"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func Test_Process(t *testing.T) {
	processor := Processor{}

	const azureMongoResourceID1 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.DocumentDB/mongo/mongodb1"
	const azureMongoResourceID2 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.DocumentDB/mongo/mongodb2"

	const host = "test.mongo.cosmos.azure.com"
	const port = 10255
	const username = "testuser"
	const password = "testpassword"
	const database = "authdb"
	const connectionString = "mongodb://testuser:testpassword@test.mongo.cosmos.azure.com:10255/authdb"

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.MongoDatabase{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureMongoResourceID1,
				},
				Values: map[string]any{
					"host":     host,
					"port":     port,
					"database": database,
					"username": username,
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(port),
			"database": database,
			"username": username,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"connectionString": {
				Value: connectionString,
			},
			"password": {
				Value: password,
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.MongoDatabase{
			Properties: datamodel.MongoDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureMongoResourceID1}},
				Host:      host,
				Port:      port,
				Database:  database,
				Username:  username,
				Secrets: datamodel.MongoDatabaseSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(port),
			"database": database,
			"username": username,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: password,
			},
			"connectionString": {
				Value: connectionString,
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromResourcesField([]*linkrp.ResourceReference{
			{
				ID: azureMongoResourceID1,
			},
		})
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.MongoDatabase{
			Properties: datamodel.MongoDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureMongoResourceID1}},
				Host:      host,
				Port:      port,
				Database:  database,
				Username:  username,
				Secrets: datamodel.MongoDatabaseSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureMongoResourceID2,
				},
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"host":     "asdf",
					"port":     3333,
					"database": "asdf",
					"username": username,
				},
				Secrets: map[string]any{
					"password":         "asdf",
					"connectionString": "asdf",
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(port),
			"database": database,
			"username": username,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: password,
			},
			"connectionString": {
				Value: connectionString,
			},
		}

		expectedOutputResources := []rpv1.OutputResource{}

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		resourceFieldOutputResources, err := processors.GetOutputResourcesFromResourcesField([]*linkrp.ResourceReference{
			{
				ID: azureMongoResourceID1,
			},
		})
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, resourceFieldOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.MongoDatabase{}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `validation returned multiple errors:

the connection value "host" should be provided by the recipe, set '.properties.host' to provide a value manually
the connection value "port" should be provided by the recipe, set '.properties.port' to provide a value manually
the connection value "database" should be provided by the recipe, set '.properties.database' to provide a value manually`, err.Error())
	})
}
