/*
Copyright 2026 The Radius Authors.

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

package deployment

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelegationPlannerUsesResourceEnvironment(t *testing.T) {
	t.Parallel()
	const environment = "/planes/radius/local/resourceGroups/test/providers/Radius.Core/environments/preview"
	for _, tt := range []struct {
		name       string
		value      string
		parameters clients.DeploymentParameters
		defaults   map[string]any
		module     bool
	}{
		{name: "literal", value: environment},
		{name: "parameter overrides workspace and default", value: "[parameters('Environment')]", parameters: clients.DeploymentParameters{
			"environment": {"value": environment},
		}, defaults: map[string]any{"Environment": map[string]any{"type": "string", "defaultValue": "wrong-default"}}},
		{name: "parameter default", value: "[parameters('environment')]", defaults: map[string]any{
			"environment": map[string]any{"type": "string", "defaultValue": environment},
		}},
		{name: "module parameter", value: "[parameters('childEnvironment')]", parameters: clients.DeploymentParameters{
			"environment": {"value": environment},
		}, module: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resourceTypes := fake.ResourceTypesServer{
				Get: func(ctx context.Context, plane, namespace, name string, options *v20231001preview.ResourceTypesClientGetOptions) (response azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResponse azfake.ErrorResponder) {
					require.Equal(t, "Radius.Compute", namespace)
					require.Equal(t, "containers", name)
					response.SetResponse(http.StatusOK, v20231001preview.ResourceTypesClientGetResponse{
						Properties: &v20231001preview.ResourceTypeProperties{},
					}, nil)
					return
				},
			}

			ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &arm.ClientOptions{
				Transport: fake.NewResourceTypesServerTransport(&resourceTypes),
			})
			require.NoError(t, err)
			loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
			loader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, metadata *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
					require.Equal(t, environment, metadata.EnvironmentID)
					return &recipes.EnvironmentDefinition{Driver: "terraform"}, nil
				},
			)
			template := map[string]any{
				"imports":    map[string]any{"radius": map[string]any{"provider": "Radius"}},
				"parameters": tt.defaults,
				"resources": map[string]any{"server": map[string]any{
					"import": "radius", "type": "Radius.Compute/containers@2025-08-01-preview",
					"properties": map[string]any{"name": "server", "properties": map[string]any{"environment": tt.value}},
				}},
			}
			if tt.module {
				template = map[string]any{"resources": map[string]any{"child": map[string]any{
					"type": NestedModuleType,
					"properties": map[string]any{
						"parameters": map[string]any{"childEnvironment": map[string]any{"value": "[parameters('environment')]"}},
						"template":   template,
					},
				}}}
			}
			planned, err := NewDelegationPlanner(loader, ucp, nil)(t.Context(), clients.DeploymentOptions{
				Template: template, Parameters: tt.parameters,
				Providers: &clients.Providers{Radius: &clients.RadiusProvider{
					EnvironmentID: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/workspace-default",
				}},
			}, "/planes/radius/local/resourceGroups/test")
			require.NoError(t, err)
			require.Equal(t, template, planned, "ordinary resources must not be rewritten")
		})
	}
}

func TestDelegationPlannerPreservesStaticRPResources(t *testing.T) {
	t.Parallel()
	for _, resourceType := range []string{"Radius.Core/applications", "Applications.Core/containers", "Applications.Dapr/stateStores"} {
		t.Run(resourceType, func(t *testing.T) {
			t.Parallel()
			template := map[string]any{
				"imports": map[string]any{"radius": map[string]any{"provider": "Radius"}},
				"resources": map[string]any{"resource": map[string]any{
					"import": "radius", "type": resourceType + "@2023-10-01-preview",
				}},
			}
			planned, err := NewDelegationPlanner(nil, nil, nil)(t.Context(), clients.DeploymentOptions{
				Template:  template,
				Providers: &clients.Providers{Radius: &clients.RadiusProvider{EnvironmentID: "workspace-environment"}},
			}, "/planes/radius/local/resourceGroups/test")
			require.NoError(t, err)
			require.Equal(t, template, planned)
		})
	}
}

func TestDelegationPlannerReadsRecipePackCreatedInDeployment(t *testing.T) {
	t.Parallel()
	resourceTypes := fake.ResourceTypesServer{
		Get: func(ctx context.Context, plane, namespace, name string, options *v20231001preview.ResourceTypesClientGetOptions) (response azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResponse azfake.ErrorResponder) {
			response.SetResponse(http.StatusOK, v20231001preview.ResourceTypesClientGetResponse{
				Properties: &v20231001preview.ResourceTypeProperties{},
			}, nil)
			return
		},
	}
	ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &arm.ClientOptions{
		Transport: fake.NewResourceTypesServerTransport(&resourceTypes),
	})
	require.NoError(t, err)
	template := map[string]any{
		"imports": map[string]any{"radius": map[string]any{"provider": "Radius"}},
		"resources": map[string]any{
			"pack": map[string]any{
				"import": "radius", "type": "Radius.Core/recipePacks@2025-08-01-preview",
				"properties": map[string]any{"name": "pack", "properties": map[string]any{
					"recipes": map[string]any{"Test.Resources/userTypeAlpha": map[string]any{
						"kind": "bicep", "source": "[parameters('source')]",
					}},
				}},
			},
			"env": map[string]any{
				"import": "radius", "type": "Radius.Core/environments@2025-08-01-preview",
				"properties": map[string]any{"name": "env", "properties": map[string]any{
					"recipePacks": []any{"[reference('pack').id]"},
				}},
			},
			"resource": map[string]any{
				"import": "radius", "type": "Test.Resources/userTypeAlpha@2023-10-01-preview",
				"properties": map[string]any{"name": "resource", "properties": map[string]any{
					"environment": "[reference('env').id]",
				}},
			},
		},
	}
	for _, reference := range []string{"[reference('pack').id]", "[reference('pack').name]", "pack"} {
		t.Run(reference, func(t *testing.T) {
			env := template["resources"].(map[string]any)["env"].(map[string]any)
			env["properties"].(map[string]any)["properties"].(map[string]any)["recipePacks"] = []any{reference}
			loader := configloader.NewMockConfigurationLoader(gomock.NewController(t))
			planned, err := NewDelegationPlanner(loader, ucp, nil)(t.Context(), clients.DeploymentOptions{
				Template:  template,
				Providers: &clients.Providers{Radius: &clients.RadiusProvider{EnvironmentID: "workspace-environment"}},
			}, "/planes/radius/local/resourceGroups/test")
			require.NoError(t, err)
			require.Equal(t, template, planned, "new environments and ordinary recipe packs must remain engine-evaluated")
		})
	}
}

func TestDelegationPlannerDoesNotResolveRecipesForExistingResources(t *testing.T) {
	t.Parallel()
	template := map[string]any{
		"imports": map[string]any{"radius": map[string]any{"provider": "Radius"}},
		"resources": map[string]any{"database": map[string]any{
			"import": "radius", "type": "Test.Resources/postgres@2025-01-01-preview",
			"existing": true, "properties": map[string]any{"name": "existing-postgres"},
		}},
	}
	planned, err := NewDelegationPlanner(nil, nil, nil)(t.Context(), clients.DeploymentOptions{
		Template:  template,
		Providers: &clients.Providers{Radius: &clients.RadiusProvider{EnvironmentID: "workspace-environment"}},
	}, "/planes/radius/local/resourceGroups/test")
	require.NoError(t, err)
	require.Equal(t, template, planned)
}
