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

	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/messagingrp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func Test_N_Process(t *testing.T) {
	processor := N_Processor{}

	const queue = "test-queue"
	const connectionString = "connection://string"
	rabbitMQOutputResources := []string{
		"/planes/kubernetes/local/namespaces/rabbitmq/providers/core/Service/rabbitmq-svc",
		"/planes/kubernetes/local/namespaces/rabbitmq/providers/apps/Deployment/rabbitmq-deployment",
	}

	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.RabbitMQQueue{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: rabbitMQOutputResources,
				Values: map[string]any{
					"queue": queue,
				},
				Secrets: map[string]any{
					"connectionString": connectionString,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, queue, resource.Properties.Queue)
		expectedValues := map[string]any{
			"queue": queue,
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.RabbitMQQueue{
			Properties: datamodel.RabbitMQQueueProperties{
				Queue: queue,
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, queue, resource.Properties.Queue)

		expectedValues := map[string]any{
			"queue": queue,
		}
		require.NoError(t, err)
		require.Equal(t, expectedValues, resource.ComputedValues)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.RabbitMQQueue{
			Properties: datamodel.RabbitMQQueueProperties{
				Queue: queue,
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: rabbitMQOutputResources,
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"queue": queue,
				},
				Secrets: map[string]any{
					"connectionString": connectionString,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, queue, resource.Properties.Queue)

		expectedValues := map[string]any{
			"queue": queue,
		}
		expectedOutputResources := []rpv1.OutputResource{}

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.RabbitMQQueue{}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `the connection value "queue" should be provided by the recipe, set '.properties.queue' to provide a value manually`, err.Error())
	})
}
