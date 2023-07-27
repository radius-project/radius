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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_display(t *testing.T) {
	t.Run("empty graph", func(t *testing.T) {
		graph := &applicationGraph{
			ApplicationName: "test-app",
			Resources:       map[string]resourceEntry{},
		}

		expected := `Displaying application: test-app

(empty)

`
		actual := display(graph)
		require.Equal(t, expected, actual)
	})

	t.Run("complex application", func(t *testing.T) {
		graph := &applicationGraph{
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

		expected := `Displaying application: test-app

Name: webapp (Applications.Core/containers)
Connections:
  webapp -> a (Applications.Datastores/redisCaches)
  webapp -> b (Applications.Datastores/redisCaches)
  webapp -> redis (Microsoft.Cache/Redis)
  webapp -> error ('asdf-invalid-YO' is not a valid resource id)
Resources:
  demo (kubernetes: apps/Deployment)

Name: a (Applications.Datastores/redisCaches)
Connections:
  webapp (Applications.Core/containers) -> a
Resources:
  redis (azure: Microsoft.Cache/Redis) %s

Name: b (Applications.Datastores/redisCaches)
Connections:
  webapp (Applications.Core/containers) -> b
Resources:
  redis-aqbjixghynqgg (aws: AWS.MemoryDB/Cluster)

Name: redis (Microsoft.Cache/Redis)
Connections:
  webapp (Applications.Core/containers) -> redis
Resources: (none)

`

		// The link contains escape sequences that we can't write in an `` string.
		link := makeHyperlink(graph.Resources[makeRedisResourceID("a")].Resources[0])
		expected = fmt.Sprintf(expected, link)

		actual := display(graph)
		require.Equal(t, expected, actual)
	})
}
