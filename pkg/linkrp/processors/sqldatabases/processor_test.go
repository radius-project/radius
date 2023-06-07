// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func Test_Process(t *testing.T) {
	processor := Processor{}

	const azureSqlResourceID = "/subscriptions/85716382-7362-45c3-ae03-2126e459a123/resourceGroups/RadiusFunctionalTest/providers/Microsoft.Sql/servers/mssql-radiustest/databases/database-radiustest"
	const server = "sql.server"
	const database = "database-radiustest"

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
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.SqlDatabase{
			Properties: datamodel.SqlDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureSqlResourceID}},
				Database:  database,
				Server:    server,
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromResourcesField([]*linkrp.ResourceReference{
			{
				ID: azureSqlResourceID,
			},
		})
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.SqlDatabase{
			Properties: datamodel.SqlDatabaseProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureSqlResourceID}},
				Database:  database,
				Server:    server,
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureSqlResourceID,
				},
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"datbqse": "override-database",
					"server":  "override.server",
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, database, resource.Properties.Database)
		require.Equal(t, server, resource.Properties.Server)

		expectedValues := map[string]any{
			"database": database,
			"server":   server,
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
the connection value "server" should be provided by the recipe, set '.properties.server' to provide a value manually`, err.Error())

	})
}
