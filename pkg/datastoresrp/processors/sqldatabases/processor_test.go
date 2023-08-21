// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp"
	"github.com/radius-project/radius/pkg/linkrp/processors"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func Test_Process(t *testing.T) {
	processor := Processor{}

	const azureSqlResourceID = "/subscriptions/85716382-7362-45c3-ae03-2126e459a123/resourceGroups/RadiusFunctionalTest/providers/Microsoft.Sql/servers/mssql-radiustest/databases/database-radiustest"
	const server = "sql.server"
	const database = "database-radiustest"
	const port = 1433
	const username = "testuser"
	const password = "testpassword"
	const connectionString = "Data Source=tcp:sql.server,1433;Initial Catalog=database-radiustest;User Id=testuser;Password=testpassword;Encrypt=True;TrustServerCertificate=True"

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.SqlDatabase{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureSqlResourceID,
				},
				Values: map[string]any{
					"database": database,
					"server":   server,
					"port":     port,
					"username": username,
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
			"port":     int32(port),
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
		resource := &datamodel.SqlDatabase{
			Properties: datamodel.SqlDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureSqlResourceID}},
				Database:  database,
				Server:    server,
				Port:      port,
				Username:  username,
				Secrets: datamodel.SqlDatabaseSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
			"port":     int32(port),
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
				ID: azureSqlResourceID,
			},
		})
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.SqlDatabase{
			Properties: datamodel.SqlDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureSqlResourceID}},
				Database:  database,
				Server:    server,
				Port:      port,
				Username:  username,
				Secrets: datamodel.SqlDatabaseSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureSqlResourceID,
				},
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"database": "override-database",
					"server":   "override.server",
					"port":     3333,
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

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)
		require.Equal(t, int32(port), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
			"port":     int32(port),
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
				ID: azureSqlResourceID,
			},
		})
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, resourceFieldOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.SqlDatabase{}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `validation returned multiple errors:

the connection value "database" should be provided by the recipe, set '.properties.database' to provide a value manually
the connection value "server" should be provided by the recipe, set '.properties.server' to provide a value manually
the connection value "port" should be provided by the recipe, set '.properties.port' to provide a value manually`, err.Error())

	})
}
