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
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// NewDelegationPlanner uses registered types and effective recipes, not resource PUT arrival order.
func NewDelegationPlanner(loader configloader.ConfigurationLoader, ucp *v20231001preview.ClientFactory) func(context.Context, clients.DeploymentOptions, string) (map[string]any, error) {
	return func(ctx context.Context, options clients.DeploymentOptions, scope string) (map[string]any, error) {
		if options.Providers == nil || options.Providers.Radius == nil || options.Providers.Radius.EnvironmentID == "" {
			return options.Template, nil
		}
		environment := options.Providers.Radius.EnvironmentID
		definitions := map[string]*recipes.EnvironmentDefinition{}
		schemas := map[string]map[string]any{}
		var load func(map[string]any, string) error
		load = func(template map[string]any, path string) error {
			declarations, _ := template["resources"].(map[string]any)
			imports, _ := template["imports"].(map[string]any)
			for symbol, raw := range declarations {
				declaration, _ := raw.(map[string]any)
				if child := delegationModuleTemplate(declaration); child != nil {
					if err := load(child, path+symbol+"/"); err != nil {
						return err
					}
					continue
				}
				importName, _ := declaration["import"].(string)
				importDefinition, _ := imports[importName].(map[string]any)
				provider, _ := importDefinition["provider"].(string)
				if !strings.HasPrefix(strings.ToLower(provider), "radius") {
					continue
				}
				resourceType, apiVersion, _ := strings.Cut(fmt.Sprint(declaration["type"]), "@")
				namespace, name, _ := strings.Cut(resourceType, "/")
				if strings.EqualFold(namespace, "Radius.Core") {
					// Core resources are handled by Applications RP rather than a provisioning recipe.
					continue
				}
				details, err := ucp.NewResourceTypesClient().Get(ctx, "local", namespace, name, nil)
				if err != nil {
					return fmt.Errorf("failed to load deployment resource type %q: %w", resourceType, err)
				}
				manual := false
				for _, capability := range details.Properties.Capabilities {
					if capability != nil && *capability == datamodel.CapabilityManualResourceProvisioning {
						manual = true
					}
				}
				if manual {
					continue
				}
				body, _ := declaration["properties"].(map[string]any)
				properties, _ := body["properties"].(map[string]any)
				resourceEnvironment := environment
				if value, ok := properties["environment"].(string); ok && !strings.HasPrefix(value, "[") {
					resourceEnvironment = value
				}
				metadata := recipes.ResourceMetadata{
					ResourceID:    scope + "/providers/" + resourceType + "/deployment-planning",
					EnvironmentID: resourceEnvironment,
				}
				definition, err := loader.LoadRecipe(ctx, &metadata)
				if err != nil {
					return fmt.Errorf("failed to load deployment recipe for %q: %w", resourceType, err)
				}
				definitions[path+symbol] = definition
				if definition.Driver == "delegated" {
					propertiesSchema, err := schema.GetSchema(ctx, ucp, metadata.ResourceID, resourceType, apiVersion)
					if err != nil {
						return fmt.Errorf("failed to load typed delegation relationships for %q: %w", resourceType, err)
					}
					schemas[path+symbol] = propertiesSchema
				}
			}
			return nil
		}
		if err := load(options.Template, ""); err != nil {
			return nil, err
		}
		return PlanDelegatedResources(options.Template, scope, definitions, schemas)
	}
}
