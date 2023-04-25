// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func Test_Process(t *testing.T) {
	processor := Processor{}

	const azureRedisResourceID1 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.Cache/redis/myredis1"
	const azureRedisResourceID2 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.Cache/redis/myredis2"
	const host = "myredis.redis.cache.windows.net"
	const connectionString = "myredis.redis.cache.windows.net:6380,abortConnect=False,ssl=True,user=testuser,password=testpassword"
	const username = "testuser"
	const password = "testpassword"

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.RedisCache{}
		output := &recipes.RecipeOutput{
			Resources: []string{
				azureRedisResourceID1,
			},
			Values: map[string]any{
				"host":     host,
				"port":     RedisSSLPort,
				"username": username,
			},
			Secrets: map[string]any{
				"password": password,

				// Let the connection string be computed, it will result in the same value
				// as the variable 'connectionString'
			},
		}

		err := processor.Process(context.Background(), resource, output)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisSSLPort),
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

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(output)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - values", func(t *testing.T) {
		resource := &datamodel.RedisCache{
			Properties: datamodel.RedisCacheProperties{
				RedisResourceProperties: datamodel.RedisResourceProperties{
					Resource: azureRedisResourceID1,
				},
				RedisValuesProperties: datamodel.RedisValuesProperties{
					Host:     host,
					Port:     RedisSSLPort,
					Username: username,
				},
				Secrets: datamodel.RedisCacheSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		err := processor.Process(context.Background(), resource, nil)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisSSLPort),
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

		expectedOutputResource, err := processors.GetOutputResourceFromResourceID(azureRedisResourceID1)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, []rpv1.OutputResource{expectedOutputResource}, resource.Properties.Status.OutputResources)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.RedisCache{
			Properties: datamodel.RedisCacheProperties{
				RedisResourceProperties: datamodel.RedisResourceProperties{
					Resource: azureRedisResourceID1,
				},
				RedisValuesProperties: datamodel.RedisValuesProperties{
					Host:     host,
					Port:     RedisSSLPort,
					Username: username,
				},
				Secrets: datamodel.RedisCacheSecrets{
					Password:         password,
					ConnectionString: connectionString,
				},
			},
		}
		output := &recipes.RecipeOutput{
			Resources: []string{
				azureRedisResourceID2,
			},

			// Values and secrets will be overridden by the resource.
			Values: map[string]any{
				"host":     "asdf",
				"port":     3333,
				"username": "asdf",
			},
			Secrets: map[string]any{
				"password":         "asdf",
				"connectionString": "asdf",
			},
		}

		err := processor.Process(context.Background(), resource, output)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisSSLPort),
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

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(output)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		resourceFieldOutputResource, err := processors.GetOutputResourceFromResourceID(azureRedisResourceID1)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, resourceFieldOutputResource)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.RedisCache{}
		output := &recipes.RecipeOutput{}

		err := processor.Process(context.Background(), resource, output)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `validation returned multiple errors:

the connection value "host" should be provided by the recipe, set '.properties.host' to provide a value manually
the connection value "port" should be provided by the recipe, set '.properties.port' to provide a value manually`, err.Error())
	})
}
