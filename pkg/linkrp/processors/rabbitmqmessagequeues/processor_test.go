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
package rabbitmqmessagequeues

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

	const queue = "test-queue"
	const uri = "connection://string"
	const host = "test-host"
	const vHost = "test-vHost"
	const port int32 = 5672
	const username = "test-user"
	const password = "test-password"
	rabbitMQOutputResources := []string{
		"/planes/kubernetes/local/namespaces/rabbitmq/providers/core/Service/rabbitmq-svc",
		"/planes/kubernetes/local/namespaces/rabbitmq/providers/apps/Deployment/rabbitmq-deployment",
	}

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.RabbitMQMessageQueue{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: rabbitMQOutputResources,
				Values: map[string]any{
					"queue":    queue,
					"host":     host,
					"port":     port,
					"username": username,
					"vHost":    vHost,
					"tls":      true,
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, queue, resource.Properties.Queue)
		expectedValues := map[string]any{
			"queue":    queue,
			"host":     host,
			"port":     port,
			"username": username,
			"vHost":    vHost,
			"tls":      true,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: password,
			},
			"uri": {
				Value: "amqps://test-user:test-password@test-host:5672/test-vHost",
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.RabbitMQMessageQueue{
			Properties: datamodel.RabbitMQMessageQueueProperties{
				Queue:    queue,
				Host:     host,
				Port:     port,
				Username: username,
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, queue, resource.Properties.Queue)

		expectedValues := map[string]any{
			"queue":    queue,
			"host":     host,
			"port":     port,
			"username": username,
			"tls":      false,
		}
		require.NoError(t, err)
		require.Equal(t, expectedValues, resource.ComputedValues)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.RabbitMQMessageQueue{
			Properties: datamodel.RabbitMQMessageQueueProperties{
				Queue:    "new-queue",
				Host:     "new-host",
				Port:     int32(5671),
				Username: "new-user",
				Secrets: datamodel.RabbitMQSecrets{
					Password: "new-passoword",
					URI:      uri,
				},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: rabbitMQOutputResources,
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"queue":    queue,
					"host":     host,
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

		require.Equal(t, "new-queue", resource.Properties.Queue)

		expectedValues := map[string]any{
			"queue":    "new-queue",
			"host":     "new-host",
			"port":     int32(5671),
			"username": "new-user",
			"tls":      true,
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"password": {
				Value: "new-passoword",
			},
			"uri": {
				Value: uri,
			},
		}
		expectedOutputResources := []rpv1.OutputResource{}

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.RabbitMQMessageQueue{}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `validation returned multiple errors:

the connection value "queue" should be provided by the recipe, set '.properties.queue' to provide a value manually
the connection value "host" should be provided by the recipe, set '.properties.host' to provide a value manually
the connection value "port" should be provided by the recipe, set '.properties.port' to provide a value manually`, err.Error())

	})
}
