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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
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

	apiVersionResource, err := options.UcpClient.NewAPIVersionsClient().Get(ctx, "local", strings.Split(resource.Type, "/")[0], strings.Split(resource.Type, "/")[1], resource.InternalMetadata.UpdatedAPIVersion, nil)

	if err != nil {
		return err
	}

	resourceProps := []string{}
	schema := apiVersionResource.APIVersionResource.Properties.Schema
	if schema != nil {
		if properties, ok := schema["properties"].(map[string]any); ok {
			for key := range properties {
				resourceProps = append(resourceProps, key)
			}
		}
	}

	for key := range computedValues {
		if !contains(resourceProps, key) {
			delete(computedValues, key)
		}
	}

	for key := range secretValues {
		if !contains(resourceProps, key) {
			delete(secretValues, key)
		}
	}

	err = resource.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: outputResources, ComputedValues: computedValues, SecretValues: secretValues})
	if err != nil {
		return err
	}

	return nil
}

// Helper function to check if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
