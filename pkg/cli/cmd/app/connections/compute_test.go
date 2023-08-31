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

package connections

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_compute(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		applicationResources := []generated.GenericResource{}
		environmentResources := []generated.GenericResource{}

		actual := compute("test-app", applicationResources, environmentResources)

		expected := &applicationGraph{"test-app", map[string]resourceEntry{}}
		require.Equal(t, expected, actual)
	})

	t.Run("no application resources", func(t *testing.T) {
		applicationResources := []generated.GenericResource{}
		environmentResources := []generated.GenericResource{
			{
				ID:         to.Ptr(redisResourceID),
				Properties: makeResourceProperties(nil, []any{redisAWSOutputResource}),
			},
		}

		actual := compute("test-app", applicationResources, environmentResources)

		expected := &applicationGraph{"test-app", map[string]resourceEntry{}}
		require.Equal(t, expected, actual)
	})

	t.Run("no connections", func(t *testing.T) {
		applicationResources := []generated.GenericResource{
			{
				ID:         to.Ptr(containerResourceID),
				Properties: makeResourceProperties(nil, []any{containerDeploymentOutputResource}),
			},
		}
		environmentResources := []generated.GenericResource{
			{
				ID:         to.Ptr(redisResourceID),
				Properties: makeResourceProperties(nil, []any{redisAWSOutputResource}),
			},
		}
		// Application resources are also part of the environment.
		environmentResources = append(environmentResources, applicationResources...)

		actual := compute("test-app", applicationResources, environmentResources)

		expected := &applicationGraph{"test-app", map[string]resourceEntry{
			containerResourceID: {
				node:        nodeFromID(containerResourceID),
				Connections: []connectionEntry{},
				Resources: []outputResourceEntry{
					{
						node: node{
							Name: "demo",
							Type: "apps/Deployment",
							ID:   "/planes/kubernetes/local/namespaces/default-demo/providers/apps/Deployment/demo",
						},
						Provider: "kubernetes",
					},
				},
			},
		}}
		require.Equal(t, expected, actual)
	})

	t.Run("connections to multiple", func(t *testing.T) {
		applicationResources := []generated.GenericResource{
			{
				ID: to.Ptr(containerResourceID),
				Properties: makeResourceProperties(
					map[string]string{
						"A": makeRedisResourceID("a"),
						"B": makeRedisResourceID("b"),
						"C": azureRedisCacheResourceID, // Direct connection to cloud resource
						"D": "asdf-invalid-YO",         // Invalid connection
					},
					[]any{containerDeploymentOutputResource}),
			},
			{
				ID:         to.Ptr(makeRedisResourceID("a")),
				Properties: makeResourceProperties(nil, []any{redisAzureOutputResource}),
			},
		}
		environmentResources := []generated.GenericResource{
			{
				ID:         to.Ptr(makeRedisResourceID("b")),
				Properties: makeResourceProperties(nil, []any{redisAWSOutputResource}),
			},
		}
		// Application resources are also part of the environment.
		environmentResources = append(environmentResources, applicationResources...)

		actual := compute("test-app", applicationResources, environmentResources)

		expected := &applicationGraph{
			ApplicationName: "test-app",
			Resources: map[string]resourceEntry{
				containerResourceID: {
					node: nodeFromID(containerResourceID),
					Connections: []connectionEntry{
						{
							Name: "A",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(makeRedisResourceID("a")),
						},
						{
							Name: "B",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(makeRedisResourceID("b")),
						},
						{
							Name: "C",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(azureRedisCacheResourceID),
						},
						{
							Name: "D",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID("asdf-invalid-YO"),
						},
					},
					Resources: []outputResourceEntry{
						{
							node: node{
								Name: "demo",
								Type: "apps/Deployment",
								ID:   "/planes/kubernetes/local/namespaces/default-demo/providers/apps/Deployment/demo",
							},
							Provider: "kubernetes",
						},
					},
				},
				makeRedisResourceID("a"): {
					node: nodeFromID(makeRedisResourceID("a")),
					Connections: []connectionEntry{
						{
							Name: "A",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(makeRedisResourceID("a")),
						},
					},
					Resources: []outputResourceEntry{
						{
							node:     nodeFromID(azureRedisCacheResourceID),
							Provider: "azure",
						},
					},
				},
				makeRedisResourceID("b"): {
					node: nodeFromID(makeRedisResourceID("b")),
					Connections: []connectionEntry{
						{
							Name: "B",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(makeRedisResourceID("b")),
						},
					},
					Resources: []outputResourceEntry{
						{
							node:     nodeFromID(awsMemoryDBResourceID),
							Provider: "aws",
						},
					},
				},
				azureRedisCacheResourceID: {
					node: nodeFromID(azureRedisCacheResourceID),
					Connections: []connectionEntry{
						{
							Name: "C",
							From: nodeFromID(containerResourceID),
							To:   nodeFromID(azureRedisCacheResourceID),
						},
					},
				},
			},
		}
		require.Equal(t, expected, actual)
	})
}

func Test_nodeFromID(t *testing.T) {
	t.Run("parse valid resource ID", func(t *testing.T) {
		node := nodeFromID(applicationResourceID)
		require.Equal(t, applicationResourceID, node.ID)
		require.Equal(t, "test-app", node.Name)
		require.Equal(t, "Applications.Core/applications", node.Type)
		require.Equal(t, "", node.Error)
	})

	t.Run("parse invalid resource ID", func(t *testing.T) {
		node := nodeFromID("\ndkdkfkdfs\t")
		require.Equal(t, "", node.ID)
		require.Equal(t, "", node.Name)
		require.Equal(t, "", node.Type)
		require.Equal(t, "'\ndkdkfkdfs\t' is not a valid resource id", node.Error)
	})
}

func Test_outputResourcesFromAPIData(t *testing.T) {
	t.Run("parse valid output resources", func(t *testing.T) {
		outputResources := []any{
			redisAWSOutputResource,
			redisAzureOutputResource,
			containerDeploymentOutputResource,
		}
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: makeResourceProperties(nil, outputResources),
		}

		// Output is always sorted.
		expected := []outputResourceEntry{
			{
				node:     nodeFromID(awsMemoryDBResourceID),
				Provider: "aws",
			},
			{
				node:     nodeFromID(azureRedisCacheResourceID),
				Provider: "azure",
			},
			{
				node: node{
					Name: "demo",
					Type: "apps/Deployment",
					ID:   "/planes/kubernetes/local/namespaces/default-demo/providers/apps/Deployment/demo",
				},
				Provider: "kubernetes",
			},
		}

		actual := outputResourcesFromAPIData(resource)
		require.Equal(t, expected, actual)
	})

	t.Run("parse invalid output resources", func(t *testing.T) {
		// An invalid output resource doesn't prevent other output resources from being parsed.
		outputResources := []any{
			redisAWSOutputResource,
			makeOutputResource("asdf-invalid-YO"),
			containerDeploymentOutputResource,
		}
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: makeResourceProperties(nil, outputResources),
		}

		// Output is always sorted.
		expected := []outputResourceEntry{
			{
				node: node{
					Error: "failed to unmarshal JSON, value was not a valid resource ID: 'asdf-invalid-YO' is not a valid resource id",
				},
			},
			{
				node:     nodeFromID(awsMemoryDBResourceID),
				Provider: "aws",
			},
			{
				node: node{
					Name: "demo",
					Type: "apps/Deployment",
					ID:   "/planes/kubernetes/local/namespaces/default-demo/providers/apps/Deployment/demo",
				},
				Provider: "kubernetes",
			},
		}

		actual := outputResourcesFromAPIData(resource)
		require.Equal(t, expected, actual)
	})

	t.Run("no status", func(t *testing.T) {
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: map[string]any{},
		}

		expected := []outputResourceEntry{}

		actual := outputResourcesFromAPIData(resource)
		require.Equal(t, expected, actual)
	})

	t.Run("no output resources", func(t *testing.T) {
		outputResources := []any{}
		resource := generated.GenericResource{
			ID: to.Ptr(containerResourceID),
			Properties: map[string]any{
				"status": map[string]any{
					"outputResources": outputResources,
				},
			},
		}

		expected := []outputResourceEntry{}

		actual := outputResourcesFromAPIData(resource)
		require.Equal(t, expected, actual)
	})

	t.Run("non-array output resources", func(t *testing.T) {
		resource := generated.GenericResource{
			ID: to.Ptr(containerResourceID),
			Properties: map[string]any{
				"status": map[string]any{
					"outputResources": "hey there, this is not an array",
				},
			},
		}

		expected := []outputResourceEntry{}

		actual := outputResourcesFromAPIData(resource)
		require.Equal(t, expected, actual)
	})
}

func Test_connectionsFromAPIData(t *testing.T) {
	t.Run("parse valid connections", func(t *testing.T) {
		connections := map[string]string{
			"A": makeRedisResourceID("a"),
			"B": makeRedisResourceID("b"),
			"C": makeRedisResourceID("c"),
		}
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: makeResourceProperties(connections, nil),
		}

		// Output is always sorted.
		expected := []connectionEntry{
			{
				Name: "A",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID(makeRedisResourceID("a")),
			},
			{
				Name: "B",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID(makeRedisResourceID("b")),
			},
			{
				Name: "C",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID(makeRedisResourceID("c")),
			},
		}

		actual := connectionsFromAPIData(resource)
		require.Equal(t, expected, actual)
	})

	t.Run("parse invalid connections", func(t *testing.T) {
		connections := map[string]string{
			"A": makeRedisResourceID("a"),
			"B": "asdf-invalid-YO",
			"C": makeRedisResourceID("c"),
		}
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: makeResourceProperties(connections, nil),
		}

		expected := []connectionEntry{
			{
				Name: "A",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID(makeRedisResourceID("a")),
			},
			{
				Name: "B",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID("asdf-invalid-YO"),
			},
			{
				Name: "C",
				From: nodeFromID(containerResourceID),
				To:   nodeFromID(makeRedisResourceID("c")),
			},
		}

		actual := connectionsFromAPIData(resource)
		require.Equal(t, expected, actual)

		// A single failure does not stop connections from being found.
		require.Equal(t, "'asdf-invalid-YO' is not a valid resource id", actual[1].To.Error)
	})

	t.Run("no connections", func(t *testing.T) {
		resource := generated.GenericResource{
			ID:         to.Ptr(containerResourceID),
			Properties: map[string]any{},
		}

		actual := connectionsFromAPIData(resource)
		require.Equal(t, []connectionEntry{}, actual)
	})

	t.Run("non-map connections", func(t *testing.T) {
		t.Run("no connections", func(t *testing.T) {
			resource := generated.GenericResource{
				ID: to.Ptr(containerResourceID),
				Properties: map[string]any{
					"connections": "hey there, this is not a map!",
				},
			}

			actual := connectionsFromAPIData(resource)
			require.Equal(t, []connectionEntry{}, actual)
		})
	})
}
