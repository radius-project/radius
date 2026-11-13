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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// NewDelegationPlanner uses registered types and effective recipes, not resource PUT arrival order.
func NewDelegationPlanner(loader configloader.ConfigurationLoader, ucp *v20231001preview.ClientFactory, armOptions *arm.ClientOptions) func(context.Context, clients.DeploymentOptions, string) (map[string]any, error) {
	return func(ctx context.Context, options clients.DeploymentOptions, scope string) (map[string]any, error) {
		if options.Providers == nil || options.Providers.Radius == nil || options.Providers.Radius.EnvironmentID == "" {
			return options.Template, nil
		}
		environment := options.Providers.Radius.EnvironmentID
		definitions := map[string]*recipes.EnvironmentDefinition{}
		schemas := map[string]map[string]any{}
		var load func(map[string]any, string, clients.DeploymentParameters) error
		load = func(template map[string]any, path string, parameters clients.DeploymentParameters) error {
			declarations, _ := template["resources"].(map[string]any)
			imports, _ := template["imports"].(map[string]any)
			for symbol, raw := range declarations {
				declaration, _ := raw.(map[string]any)
				if child := delegationModuleTemplate(declaration); child != nil {
					childParameters := clients.DeploymentParameters{}
					moduleProperties, _ := declaration["properties"].(map[string]any)
					arguments, _ := moduleProperties["parameters"].(map[string]any)
					for name, raw := range arguments {
						argument, _ := raw.(map[string]any)
						if value, ok := deploymentMetadataString(argument["value"], template, parameters); ok {
							childParameters[name] = bicep.NewParameter(value)
						}
					}
					if err := load(child, path+symbol+"/", childParameters); err != nil {
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
				if strings.EqualFold(namespace, "Radius.Core") || strings.HasPrefix(strings.ToLower(namespace), "applications.") {
					// Static RPs retain their existing provisioning paths; grouping is handled by Dynamic RP.
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
				if value, ok := deploymentMetadataString(properties["environment"], template, parameters); ok {
					resourceEnvironment = value
				}
				metadata := recipes.ResourceMetadata{
					ResourceID:    scope + "/providers/" + resourceType + "/deployment-planning",
					EnvironmentID: resourceEnvironment,
				}
				definition, declared, err := deploymentDeclaredRecipe(ctx, template, properties["environment"], parameters, scope, resourceType, armOptions)
				if err == nil && !declared {
					definition, err = loader.LoadRecipe(ctx, &metadata)
				}
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
		if err := load(options.Template, "", options.Parameters); err != nil {
			return nil, err
		}
		return PlanDelegatedResources(options.Template, scope, definitions, schemas)
	}
}

// deploymentMetadataString resolves literal or directly parameterized metadata, not resource outputs.
func deploymentMetadataString(value any, template map[string]any, parameters clients.DeploymentParameters) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(text, "[") {
		return text, true
	}
	if !strings.HasSuffix(text, "]") {
		return "", false
	}
	arguments := delegationARMArguments(text[1:len(text)-1], "parameters")
	if len(arguments) != 1 || !strings.HasPrefix(arguments[0], "'") || armQuotedEnd(arguments[0], 0) != len(arguments[0]) {
		return "", false
	}
	name := strings.ReplaceAll(arguments[0][1:len(arguments[0])-1], "''", "'")
	for provided, parameter := range parameters {
		if strings.EqualFold(provided, name) {
			result, ok := parameter["value"].(string)
			return result, ok
		}
	}
	declared, _ := template["parameters"].(map[string]any)
	for parameter, definition := range declared {
		if strings.EqualFold(parameter, name) {
			value, _ := bicep.DefaultValue(definition)
			result, ok := value.(string)
			return result, ok && !strings.HasPrefix(result, "[")
		}
	}
	return "", false
}

func deploymentDeclaredRecipe(ctx context.Context, template map[string]any, environmentValue any, parameters clients.DeploymentParameters, scope, resourceType string, armOptions *arm.ClientOptions) (*recipes.EnvironmentDefinition, bool, error) {
	environment, _ := environmentValue.(string)
	match := symbolicIDReference.FindStringSubmatch(environment)
	if len(match) != 2 {
		return nil, false, nil
	}
	declarations, _ := template["resources"].(map[string]any)
	declaration, _ := declarations[match[1]].(map[string]any)
	if declaration["existing"] == true || declaration["type"] != "Radius.Core/environments@2025-08-01-preview" {
		return nil, false, nil
	}
	body, _ := declaration["properties"].(map[string]any)
	properties, _ := body["properties"].(map[string]any)
	packs, _ := properties["recipePacks"].([]any)
	var definition *recipes.EnvironmentDefinition
	for _, value := range packs {
		var registrations map[string]any
		reference, _ := value.(string)
		packMatch := symbolicIDReference.FindStringSubmatch(reference)
		if len(packMatch) == 2 {
			pack, _ := declarations[packMatch[1]].(map[string]any)
			if pack["existing"] != true {
				packBody, _ := pack["properties"].(map[string]any)
				packProperties, _ := packBody["properties"].(map[string]any)
				registrations, _ = packProperties["recipes"].(map[string]any)
			} else {
				packBody, _ := pack["properties"].(map[string]any)
				name, ok := deploymentMetadataString(packBody["name"], template, parameters)
				if !ok {
					return nil, true, fmt.Errorf("cannot resolve declared recipe pack identity")
				}
				reference = scope + "/providers/Radius.Core/recipePacks/" + name
			}
		}
		if registrations != nil {
			for registeredType, raw := range registrations {
				if !strings.EqualFold(registeredType, resourceType) {
					continue
				}
				if definition != nil {
					return nil, true, fmt.Errorf("resource type %q is defined in multiple recipe packs", resourceType)
				}
				registration, _ := raw.(map[string]any)
				kind, ok := deploymentMetadataString(registration["kind"], template, parameters)
				if !ok {
					return nil, true, fmt.Errorf("cannot resolve declared recipe kind for %q", resourceType)
				}
				definition = &recipes.EnvironmentDefinition{Driver: kind}
				if target, exists := registration["delegatesTo"]; exists {
					var ok bool
					definition.DelegatesTo, ok = deploymentMetadataString(target, template, parameters)
					if !ok {
						return nil, true, fmt.Errorf("cannot resolve declared delegation target type for %q", resourceType)
					}
				}
				if accepted, exists := registration["acceptsDelegationFrom"]; exists {
					values, ok := accepted.([]any)
					if !ok {
						return nil, true, fmt.Errorf("cannot resolve declared delegation acceptance for %q", resourceType)
					}
					for _, value := range values {
						acceptedType, ok := deploymentMetadataString(value, template, parameters)
						if !ok {
							return nil, true, fmt.Errorf("cannot resolve declared delegation acceptance for %q", resourceType)
						}
						definition.AcceptsDelegationFrom = append(definition.AcceptsDelegationFrom, acceptedType)
					}
				}
			}
			continue
		}
		id, ok := deploymentMetadataString(reference, template, parameters)
		if !ok {
			return nil, true, fmt.Errorf("cannot resolve declared environment recipe pack")
		}
		pack, err := configloader.FetchRecipePack(ctx, id, armOptions)
		if err != nil {
			return nil, true, err
		}
		if pack.Properties == nil {
			return nil, true, fmt.Errorf("recipe pack %q has no properties", id)
		}
		for registeredType, registration := range pack.Properties.Recipes {
			if !strings.EqualFold(registeredType, resourceType) {
				continue
			}
			if definition != nil {
				return nil, true, fmt.Errorf("resource type %q is defined in multiple recipe packs", resourceType)
			}
			if registration == nil || registration.Kind == nil {
				return nil, true, fmt.Errorf("recipe pack %q has no recipe kind for %q", id, resourceType)
			}
			definition = &recipes.EnvironmentDefinition{Driver: string(*registration.Kind)}
			if registration.DelegatesTo != nil {
				definition.DelegatesTo = *registration.DelegatesTo
			}
			for _, accepted := range registration.AcceptsDelegationFrom {
				if accepted != nil {
					definition.AcceptsDelegationFrom = append(definition.AcceptsDelegationFrom, *accepted)
				}
			}
		}
	}
	if definition == nil {
		return nil, true, fmt.Errorf("could not find recipe for %q in declared environment", resourceType)
	}
	return definition, true, nil
}
