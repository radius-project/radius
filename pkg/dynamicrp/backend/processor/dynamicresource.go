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

package processor

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"golang.org/x/exp/slices"
)

var _ processors.ResourceProcessor[*datamodel.DynamicResource, datamodel.DynamicResource] = (*DynamicProcessor)(nil)

const connectedResourceOutputVariable = "connected-resource-environment-variable"

// DynamicProcessor is a processor for dynamic resources. It implements the processors.ResourceProcessor interface.
type DynamicProcessor struct {
}

// Delete implements the processors.Processor interface for dynamic resources.
// Deletion of resources is handled in recipe_delete_controller.go and inert_delete_controller.go.
func (d *DynamicProcessor) Delete(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	return nil
}

// Process validates resource properties, and applies output values from the recipe output.
func (d *DynamicProcessor) Process(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	computedValues := map[string]any{}
	secretValues := map[string]rpv1.SecretValueReference{}
	outputResources := []rpv1.OutputResource{}
	status := rpv1.RecipeStatus{}

	validator := processors.NewValidator(&computedValues, &secretValues, &outputResources, &status)

	// TODO: loop over schema and add to validator - right now this bypasses validation.
	for key, value := range options.RecipeOutput.Values {
		value := value
		validator.AddOptionalAnyField(key, &value)
	}
	for key, value := range options.RecipeOutput.Secrets {
		value := value.(string)
		validator.AddOptionalSecretField(key, &value)
	}

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	err = resource.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: outputResources, ComputedValues: computedValues, SecretValues: secretValues})
	if err != nil {
		return err
	}

	schema, err := getAPIVersionResourceSchema(ctx, options.UcpClient, resource)
	if err != nil {
		return err
	}

	err = addOutputValuestoResourceProperties(resource, schema, computedValues, secretValues)
	if err != nil {
		return err
	}

	err = addEnvironmentMappingToResourceProperties(resource, schema)
	if err != nil {
		return err
	}

	return nil
}

func getAPIVersionResourceSchema(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resource *datamodel.DynamicResource) (map[string]any, error) {
	ID, err := resources.Parse(resource.ID)
	if err != nil {
		return nil, err
	}

	plane := ID.PlaneNamespace()
	planeName := strings.Split(plane, "/")[1]
	resourceProvider := strings.Split(resource.Type, "/")[0]
	resourceType := strings.Split(resource.Type, "/")[1]
	apiVersionResource, err := ucpClient.NewAPIVersionsClient().Get(ctx, planeName, resourceProvider, resourceType, resource.InternalMetadata.UpdatedAPIVersion, nil)
	if err != nil {
		return nil, err
	}

	schema := apiVersionResource.APIVersionResource.Properties.Schema

	return schema, nil
}

// addOutputValuestoResourceProperties adds the computed values and secret values to the resource properties.
// It retrieves the schema of the resource type and filters out the values that are not part of the schema.
func addOutputValuestoResourceProperties(resource *datamodel.DynamicResource, schema map[string]any, computedValues map[string]any, secretValues map[string]rpv1.SecretValueReference) error {
	// Filter out the basic properties from the resource properties
	// This is to avoid overwriting the properties like application, environment etc when they are added as computed values or secret values.
	basicProperties := []string{"application", "environment", "status"}
	resourceProps := []string{}

	if schema != nil {
		if properties, ok := schema["properties"].(map[string]any); ok {
			for key := range properties {
				if !slices.Contains(basicProperties, key) {
					resourceProps = append(resourceProps, key)
				}
			}
		}
	}

	// Add the computed values to the resource properties if they are part of the schema.
	for key, value := range computedValues {
		if slices.Contains(resourceProps, key) {
			resource.Properties[key] = value
		}
	}

	// Add the secret values to the resource properties if they are part of the schema.
	for key, value := range secretValues {
		if slices.Contains(resourceProps, key) {
			resource.Properties[key] = value.Value
		}
	}

	return nil
}

func addEnvironmentMappingToResourceProperties(resource *datamodel.DynamicResource, schema map[string]any) error {
	if schema != nil {
		status := resource.Status()
		environmentVariables := map[string]string{}

		if properties, ok := schema["properties"].(map[string]any); ok {
			for key := range properties {
				attributes, ok := properties[key].(map[string]any)
				if !ok {
					return fmt.Errorf("failed to assert type for attributes of property %q", key)
				}

				envVariableName, ok := attributes[connectedResourceOutputVariable].(string)
				if !ok {
					continue
				}

				if envVariableName != "" {
					value, exists := resource.Properties[key]

					if !exists {
						return fmt.Errorf("property '%s' does not exist in resource properties", key)
					}
					if value != nil {
						// Handle pointer dereference
						if ptr, ok := value.(*interface{}); ok && ptr != nil {
							// Dereference the pointer to get the actual value
							actualValue := *ptr
							// Convert the dereferenced value to string
							environmentVariables[envVariableName] = fmt.Sprintf("%v", actualValue)
						} else {
							// If it's not a pointer or is a different type, convert directly
							environmentVariables[envVariableName] = fmt.Sprintf("%v", value)
						}
					} else {
						// Handle nil pointer case
						fmt.Println("value is nil")
					}
				}
			}
		} else {
			return fmt.Errorf("failed to assert type for 'properties' in schema")
		}

		status["outputVariables"] = environmentVariables
	}

	return nil
}
