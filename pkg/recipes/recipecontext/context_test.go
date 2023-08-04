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

package recipecontext

import (
	"testing"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func Test_ContextParameter(t *testing.T) {
	linkID := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0"
	expectedLinkContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}
	linkContext, err := New(&recipes.ResourceMetadata{
		ResourceID:    linkID,
		EnvironmentID: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		ApplicationID: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
	}, &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, expectedLinkContext, *linkContext)
}

func Test_ContextParameterError(t *testing.T) {
	linkContext, err := New(&recipes.ResourceMetadata{
		ResourceID:    "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
		EnvironmentID: "error-env",
		ApplicationID: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
	}, &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	})

	require.Error(t, err)
	require.Nil(t, linkContext)
}
