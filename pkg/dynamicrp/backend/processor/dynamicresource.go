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
	"strings"

	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/resourceutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"golang.org/x/exp/slices"
)

var _ processors.ResourceProcessor[*datamodel.DynamicResource, datamodel.DynamicResource] = (*DynamicProcessor)(nil)

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

	err = addOutputValuestoResourceProperties(ctx, options.UcpClient, resource, computedValues, secretValues)
	if err != nil {
		return err
	}

	// at this point, the duynamic resource has all properties and output properties as properties.
	// if another udt connects to it, we should retrieve all properties and pass it down to the recipe context of the connecting resource.
	// enrich recipe context type
	// while rendering the resource, we retrieve connected resource properties and pass it down to recipe engine through recipe context

	return nil
}

// addOutputValuestoResourceProperties adds the computed values and secret values to the resource properties.
// It retrieves the schema of the resource type and filters out the values that are not part of the schema.
func addOutputValuestoResourceProperties(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resource *datamodel.DynamicResource, computedValues map[string]any, secretValues map[string]rpv1.SecretValueReference) error {

	ID, err := resources.Parse(resource.ID)
	if err != nil {
		return err
	}

	plane := ID.PlaneNamespace()
	planeName := strings.Split(plane, "/")[1]
	resourceProvider := strings.Split(resource.Type, "/")[0]
	resourceType := strings.Split(resource.Type, "/")[1]
	apiVersionResource, err := ucpClient.NewAPIVersionsClient().Get(ctx, planeName, resourceProvider, resourceType, resource.InternalMetadata.UpdatedAPIVersion, nil)
	if err != nil {
		return err
	}

	// Filter out the basic properties from the resource properties
	// This is to avoid overwriting the properties like application, environment etc when they are added as computed values or secret values.
	resourceProps := []string{}
	schema := apiVersionResource.APIVersionResource.Properties.Schema
	if schema != nil {
		if properties, ok := schema["properties"].(map[string]any); ok {
			for key := range properties {
				if !slices.Contains(resourceutil.BasicProperties, key) {
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
