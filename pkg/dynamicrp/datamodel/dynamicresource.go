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

package datamodel

import (
	"encoding/json"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

var _ v1.ResourceDataModel = (*DynamicResource)(nil)
var _ rpv1.RadiusResourceModel = (*DynamicResource)(nil)
var _ datamodel.RecipeDataModel = (*DynamicResource)(nil)

// DynamicResource is used as the data model for dynamic resources (UDT).
//
// A dynamic resource uses a user-provided OpenAPI specification to define the resource schema. Therefore,
// the properties of the resource are not known at compile time.
type DynamicResource struct {
	v1.BaseResource

	// Properties stores the properties of the resource being tracked.
	Properties map[string]any `json:"properties"`
}

// Status() returns the status of the resource.
func (d *DynamicResource) Status() map[string]any {
	if d.Properties == nil {
		d.Properties = map[string]any{}
	}

	// We make the assumption that the status is a map[string]any.
	// If users define the status as something other than a map[string]any, that just won't work.
	//
	// Therefore we overwrite it.
	emptyStatus := map[string]any{}
	obj, ok := d.Properties["status"]
	if !ok {
		d.Properties["status"] = emptyStatus
		return emptyStatus
	}

	status, ok := obj.(map[string]any)
	if !ok {
		d.Properties["status"] = emptyStatus
		return emptyStatus
	}

	return status
}

// GetRecipe implements datamodel.RecipeDataModel.
func (d *DynamicResource) GetRecipe() *portableresources.ResourceRecipe {
	defaultRecipe := &portableresources.ResourceRecipe{Name: portableresources.DefaultRecipeName}

	if d.Properties == nil {
		return defaultRecipe
	}

	obj, ok := d.Properties["recipe"]
	if !ok {
		return defaultRecipe
	}

	recipe, ok := obj.(map[string]any)
	if !ok {
		return defaultRecipe
	}

	// This is the best we can do. We require all of the data we store to be JSON-marshallable,
	// and the data should have already been validated when it was set.
	bs, err := json.Marshal(recipe)
	if err != nil {
		panic("failed to marshal recipe: " + err.Error())
	}

	result := portableresources.ResourceRecipe{}
	err = json.Unmarshal(bs, &result)
	if err != nil {
		panic("failed to unmarshal recipe: " + err.Error())
	}

	return &result
}

// SetRecipe implements datamodel.RecipeDataModel.
func (d *DynamicResource) SetRecipe(recipe *portableresources.ResourceRecipe) {
	if d.Properties == nil {
		d.Properties = map[string]any{}
	}

	if recipe == nil {
		d.Properties["recipe"] = map[string]any{}
		return
	}

	// This is the best we can do. We designed the ResourceRecipe type to be JSON-marshallable.
	bs, err := json.Marshal(recipe)
	if err != nil {
		panic("failed to marshal recipe: " + err.Error())
	}

	store := map[string]any{}
	err = json.Unmarshal(bs, &store)
	if err != nil {
		panic("failed to unmarshal recipe: " + err.Error())
	}

	d.Properties["recipe"] = store
}

var _ rpv1.BasicResourcePropertiesAdapter = (*dynamicResourceBasicPropertiesAdapter)(nil)

// ApplyDeploymentOutput implements v1.RadiusResourceModel.
func (d *DynamicResource) ApplyDeploymentOutput(deploymentOutput rpv1.DeploymentOutput) error {
	status := d.Status()

	// This is the best we can do. We require all of the data we store to be JSON-marshallable.
	bs, err := json.Marshal(deploymentOutput.DeployedOutputResources)
	if err != nil {
		return fmt.Errorf("failed to marshal output resources: %w", err)
	}

	outputResources := []map[string]any{}
	err = json.Unmarshal(bs, &outputResources)
	if err != nil {
		return fmt.Errorf("failed to unmarshal output resources: %w", err)
	}

	if len(outputResources) > 0 {
		status["outputResources"] = outputResources
	}

	// Store computed values and secrets as separate maps under status.
	computedValues := map[string]any{}
	for key, value := range deploymentOutput.ComputedValues {
		computedValues[key] = value
	}
	if len(computedValues) > 0 {
		status["computedValues"] = computedValues
	}

	secrets := map[string]rpv1.SecretValueReference{}
	for key, value := range deploymentOutput.SecretValues {
		secrets[key] = value
	}
	if len(secrets) > 0 {
		status["secrets"] = secrets
	}

	return nil
}

// OutputResources implements v1.RadiusResourceModel.
func (d *DynamicResource) OutputResources() []rpv1.OutputResource {
	return d.ResourceMetadata().GetResourceStatus().OutputResources
}

// ResourceMetadata returns an adapter that provides standardized access to BasicResourceProperties of the DynamicResource instance.
func (d *DynamicResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return &dynamicResourceBasicPropertiesAdapter{resource: d}
}

// dynamicResourceBasicPropertiesAdapter adapts a DynamicResource to the BasicResourcePropertiesAdapter interface
// so it can be used with our shared controllers.
type dynamicResourceBasicPropertiesAdapter struct {
	resource *DynamicResource
}

// ApplicationID implements v1.BasicResourcePropertiesAdapter.
func (d *dynamicResourceBasicPropertiesAdapter) ApplicationID() string {
	if d.resource.Properties == nil {
		return ""
	}

	obj, ok := d.resource.Properties["application"]
	if !ok {
		return ""
	}

	str, ok := obj.(string)
	if !ok {
		return ""
	}

	return str
}

// EnvironmentID implements v1.BasicResourcePropertiesAdapter.
func (d *dynamicResourceBasicPropertiesAdapter) EnvironmentID() string {
	if d.resource.Properties == nil {
		return ""
	}

	obj, ok := d.resource.Properties["environment"]
	if !ok {
		return ""
	}

	str, ok := obj.(string)
	if !ok {
		return ""
	}

	return str
}

// GetResourceStatus implements v1.BasicResourcePropertiesAdapter.
func (d *dynamicResourceBasicPropertiesAdapter) GetResourceStatus() rpv1.ResourceStatus {
	// This is the best we can do. We require all of the data we store to be JSON-marshallable,
	// and the data should have already been validated when it was set.
	bs, err := json.Marshal(d.resource.Status())
	if err != nil {
		panic("failed to marshal status: " + err.Error())
	}

	result := rpv1.ResourceStatus{}
	err = json.Unmarshal(bs, &result)
	if err != nil {
		panic("failed to unmarshal status: " + err.Error())
	}

	return result
}

// SetResourceStatus implements v1.BasicResourcePropertiesAdapter.
func (d *dynamicResourceBasicPropertiesAdapter) SetResourceStatus(status rpv1.ResourceStatus) {
	// This is the best we can do. We designed the ResourceStatus type to be JSON-marshallable.
	bs, err := json.Marshal(status)
	if err != nil {
		panic("failed to marshal status: " + err.Error())
	}

	marshaledResourceStatus := map[string]any{}
	err = json.Unmarshal(bs, &marshaledResourceStatus)
	if err != nil {
		panic("failed to unmarshal status: " + err.Error())
	}

	if d.resource.Properties == nil {
		d.resource.Properties = map[string]any{}
	}

	// This is tricky because users are allowed to add their own fields to ".properties.status".
	// We need to do a merge instead of a simple overwrite.
	existingStatus := d.resource.Status()
	for key, value := range marshaledResourceStatus {
		existingStatus[key] = value
	}
}

// GetComputedValues returns the computed values from the status map.
func (d *DynamicResource) GetComputedValues() map[string]any {
	status := d.Status()
	computed, ok := status["computedValues"].(map[string]any)
	if !ok {
		return map[string]any{}
	}

	return computed
}

// GetSecrets returns the secrets from the status map as map[string]rpv1.SecretValueReference.
func (d *DynamicResource) GetSecrets() map[string]rpv1.SecretValueReference {
	status := d.Status()
	secretsMap := map[string]rpv1.SecretValueReference{}
	secrets, ok := status["secrets"].(map[string]any)
	if !ok {
		return secretsMap
	}
	for k, v := range secrets {
		// Handle SecretValueReference structs
		if secretRef, ok := v.(rpv1.SecretValueReference); ok {
			secretsMap[k] = secretRef
			continue
		}

		// Handle the case where SecretValueReference was JSON marshaled/unmarshaled
		// and became a map[string]any
		if secretMap, ok := v.(map[string]any); ok {
			if value, exists := secretMap["Value"]; exists {
				if valueStr, ok := value.(string); ok {
					secretsMap[k] = rpv1.SecretValueReference{Value: valueStr}
				}
			}
		}
	}

	return secretsMap
}
