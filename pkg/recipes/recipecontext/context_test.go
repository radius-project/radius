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

	coredm "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"

	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {
	testMetadata := &recipes.ResourceMetadata{
		ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
		EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
	}

	ctxTests := []struct {
		name      string
		metadata  *recipes.ResourceMetadata
		providers *recipes.Configuration
		out       *Context
	}{
		{
			name:     "all providers",
			metadata: testMetadata,
			providers: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Providers: coredm.Providers{
					Azure: coredm.ProvidersAzure{
						Scope: "/subscriptions/testSub/resourceGroups/testGroup",
					},
					AWS: coredm.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/1234567890/regions/us-west-2",
					},
				},
			},
			out: &Context{
				Resource: Resource{
					ResourceInfo: ResourceInfo{
						ID:   "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
						Name: "mongo0",
					},
					Type: "applications.link/mongodatabases",
				},
				Application: ResourceInfo{
					Name: "testApplication",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				},
				Environment: ResourceInfo{
					Name: "env0",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Azure: &ProviderAzure{
					ResourceGroup: AzureResourceGroup{
						Name: "testGroup",
						ID:   "/subscriptions/testSub/resourceGroups/testGroup",
					},
					Subscription: AzureSubscription{
						SubscriptionID: "testSub",
						ID:             "/subscriptions/testSub",
					},
				},
				AWS: &ProviderAWS{
					Region:  "us-west-2",
					Account: "1234567890",
				},
			},
		},
		{
			name:     "without cloud providers",
			metadata: testMetadata,
			providers: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Providers: coredm.Providers{},
			},
			out: &Context{
				Resource: Resource{
					ResourceInfo: ResourceInfo{
						ID:   "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
						Name: "mongo0",
					},
					Type: "applications.link/mongodatabases",
				},
				Application: ResourceInfo{
					Name: "testApplication",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				},
				Environment: ResourceInfo{
					Name: "env0",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
			},
		},
		{
			name:     "only azure",
			metadata: testMetadata,
			providers: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Providers: coredm.Providers{
					Azure: coredm.ProvidersAzure{
						Scope: "/subscriptions/testSub/resourceGroups/testGroup",
					},
				},
			},
			out: &Context{
				Resource: Resource{
					ResourceInfo: ResourceInfo{
						ID:   "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
						Name: "mongo0",
					},
					Type: "applications.link/mongodatabases",
				},
				Application: ResourceInfo{
					Name: "testApplication",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				},
				Environment: ResourceInfo{
					Name: "env0",
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Azure: &ProviderAzure{
					ResourceGroup: AzureResourceGroup{
						Name: "testGroup",
						ID:   "/subscriptions/testSub/resourceGroups/testGroup",
					},
					Subscription: AzureSubscription{
						SubscriptionID: "testSub",
						ID:             "/subscriptions/testSub",
					},
				},
			},
		},
	}

	for _, tc := range ctxTests {
		t.Run(tc.name, func(t *testing.T) {
			recipeContext, err := New(tc.metadata, tc.providers)
			require.NoError(t, err)
			require.Equal(t, tc.out, recipeContext)
		})
	}
}

func TestNewContext_failures(t *testing.T) {
	testProviders := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}
	tests := []struct {
		name      string
		metadata  *recipes.ResourceMetadata
		providers *recipes.Configuration
		err       string
	}{
		{
			name: "invalid resource id",
			metadata: &recipes.ResourceMetadata{
				ResourceID:    "invalid-env",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			providers: testProviders,
			err:       "failed to parse resourceID: \"invalid-env\" while building the recipe context parameter 'invalid-env' is not a valid resource id",
		},
		{
			name: "invalid env id",
			metadata: &recipes.ResourceMetadata{
				ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				EnvironmentID: "invalid-env",
				ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			providers: testProviders,
			err:       "failed to parse environmentID: \"invalid-env\" while building the recipe context parameter 'invalid-env' is not a valid resource id",
		},
		{
			name: "invalid app id",
			metadata: &recipes.ResourceMetadata{
				ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				ApplicationID: "invalid-app",
			},
			providers: testProviders,
			err:       "failed to parse applicationID: \"invalid-app\" while building the recipe context parameter 'invalid-app' is not a valid resource id",
		},
		{
			name: "invalid azure scope",
			metadata: &recipes.ResourceMetadata{
				ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			providers: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Providers: coredm.Providers{
					Azure: coredm.ProvidersAzure{
						Scope: "invalid",
					},
					AWS: coredm.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/1234567890/regions/us-west-2",
					},
				},
			},
			err: "failed to parse Azure scope: \"invalid\" while building the recipe context parameter 'invalid' is not a valid resource id",
		},
		{
			name: "invalid aws scope",
			metadata: &recipes.ResourceMetadata{
				ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			providers: &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace:            "radius-test-app",
						EnvironmentNamespace: "radius-test-env",
					},
				},
				Providers: coredm.Providers{
					Azure: coredm.ProvidersAzure{
						Scope: "/planes/radius/local/resourceGroups/test-group",
					},
					AWS: coredm.ProvidersAWS{
						Scope: "invalid-aws",
					},
				},
			},
			err: "failed to parse AWS scope: \"invalid-aws\" while building the recipe context parameter 'invalid-aws' is not a valid resource id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.metadata, tc.providers)
			require.ErrorContains(t, err, tc.err)
		})
	}
}
