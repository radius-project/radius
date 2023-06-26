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

package rediscaches

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

	const azureRedisResourceID1 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.Cache/redis/myredis1"
	const azureRedisResourceID2 = "/subscriptions/0000/resourceGroups/test-group/providers/Microsoft.Cache/redis/myredis2"
	const host = "myredis.redis.cache.windows.net"
	const connectionString = "myredis.redis.cache.windows.net:6380,abortConnect=False,ssl=True,user=testuser,password=testpassword"
	const connectionString_NonSSL = "myredis.redis.cache.windows.net:6379,abortConnect=False,user=testuser,password=testpassword"
	const connectionURI = "rediss://testuser:testpassword@myredis.redis.cache.windows.net:6380/0?"
	const connectionURI_NonSSL = "redis://testuser:testpassword@myredis.redis.cache.windows.net:6379/0?"
	const username = "testuser"
	const password = "testpassword"

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.RedisCache{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureRedisResourceID1,
				},
				Values: map[string]any{
					"host":     host,
					"port":     RedisNonSSLPort,
					"username": username,
				},
				Secrets: map[string]any{
					"password": password,
					// Let the connection string be computed, it will result in the same value
					// as the variable 'connectionString'
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisNonSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, false, resource.Properties.TLS)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString_NonSSL, resource.Properties.Secrets.ConnectionString)
		require.Equal(t, connectionURI_NonSSL, resource.Properties.Secrets.URL)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisNonSSLPort),
			"username": username,
			"ssl":      false,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"connectionString": {
				Value: connectionString_NonSSL,
			},
			"password": {
				Value: password,
			},
			"connectionURI": {
				Value: connectionURI_NonSSL,
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.RedisCache{
			Properties: datamodel.RedisCacheProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureRedisResourceID1}},
				Host:      host,
				Port:      RedisSSLPort,
				Username:  username,
				TLS:       true,
				Secrets: datamodel.RedisCacheSecrets{
					Password:         password,
					ConnectionString: connectionString,
					URL:              connectionURI,
				},
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)
		require.Equal(t, connectionURI, resource.Properties.Secrets.URL)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisSSLPort),
			"username": username,
			"ssl":      true,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: password,
			},
			"connectionString": {
				Value: connectionString,
			},
			"connectionURI": {
				Value: connectionURI,
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromResourcesField([]*linkrp.ResourceReference{
			{
				ID: azureRedisResourceID1,
			},
		})
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.RedisCache{
			Properties: datamodel.RedisCacheProperties{
				Resources: []*linkrp.ResourceReference{{ID: azureRedisResourceID1}},
				Host:      host,
				Port:      RedisSSLPort,
				Username:  username,

				Secrets: datamodel.RedisCacheSecrets{
					Password:         password,
					ConnectionString: connectionString,
					URL:              connectionURI,
				},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					azureRedisResourceID2,
				},
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"host":     "asdf",
					"port":     3333,
					"username": "asdf",
					"ssl":      true,
				},
				Secrets: map[string]any{
					"password":         "asdf",
					"connectionString": "asdf",
					"connectionURI":    "asdf",
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, host, resource.Properties.Host)
		require.Equal(t, int32(RedisSSLPort), resource.Properties.Port)
		require.Equal(t, username, resource.Properties.Username)
		require.Equal(t, password, resource.Properties.Secrets.Password)
		require.Equal(t, connectionString, resource.Properties.Secrets.ConnectionString)
		require.Equal(t, connectionURI, resource.Properties.Secrets.URL)

		expectedValues := map[string]any{
			"host":     host,
			"port":     int32(RedisSSLPort),
			"username": username,
			"ssl":      true,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: password,
			},
			"connectionString": {
				Value: connectionString,
			},
			"connectionURI": {
				Value: connectionURI,
			},
		}

		expectedOutputResources := []rpv1.OutputResource{}

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		resourceFieldOutputResources, err := processors.GetOutputResourcesFromResourcesField([]*linkrp.ResourceReference{
			{
				ID: azureRedisResourceID1,
			},
		})
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, resourceFieldOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.RedisCache{}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `validation returned multiple errors:

the connection value "host" should be provided by the recipe, set '.properties.host' to provide a value manually
the connection value "port" should be provided by the recipe, set '.properties.port' to provide a value manually`, err.Error())
	})
}
