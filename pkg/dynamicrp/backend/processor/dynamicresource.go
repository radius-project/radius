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
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/dynamicrp/backend/secret"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/resourceutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	schemautil "github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"golang.org/x/exp/slices"
)

var _ processors.ResourceProcessor[*datamodel.DynamicResource, datamodel.DynamicResource] = (*DynamicProcessor)(nil)
var ErrNoSchemaFound = errors.New("no schema found for resource type")

// DynamicProcessor is a processor for dynamic resources. It implements the processors.ResourceProcessor interface.
type DynamicProcessor struct {
	// SecretMaterializer materializes declared recipe secret outputs into a managed Radius.Security/secrets
	// resource. When nil, secret materialization is skipped and recipe secret outputs are dropped (never
	// persisted on the owner resource).
	SecretMaterializer secret.Materializer
}

// Delete implements the processors.Processor interface for dynamic resources.
// Deletion of the recipe-created resources is handled in recipe_delete_controller.go and
// inert_delete_controller.go. Here we cascade-delete the managed Radius.Security/secrets resource that
// backs this resource's declared secret outputs, if one was materialized.
func (d *DynamicProcessor) Delete(ctx context.Context, resource *datamodel.DynamicResource, options processors.Options) error {
	if d.SecretMaterializer == nil {
		return nil
	}
	// Only resources that materialized a managed secret carry the secret reference; skip the delete call
	// otherwise to avoid spurious requests.
	secrets, ok := resource.Properties[schemautil.SecretsBlockPropertyName].(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := secrets[schemautil.SecretNameReferenceKey]; !ok {
		return nil
	}
	return d.SecretMaterializer.Delete(ctx, resource.ID)
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
		validator.AddOptionalAnyField(key, &value)
	}
	for key, value := range options.RecipeOutput.Secrets {
		if value == nil {
			// A nil secret value carries no data; skip it so we don't record the
			// literal string "<nil>" and overwrite an otherwise-unset secret field.
			continue
		}
		// Secret values originating from a direct module's outputs may not be strings
		// (e.g. numeric or boolean Terraform outputs). Stringify defensively to avoid a panic.
		strValue, ok := value.(string)
		if !ok {
			strValue = fmt.Sprintf("%v", value)
		}
		validator.AddOptionalSecretField(key, &strValue)
	}

	if err := validator.SetAndValidate(options.RecipeOutput); err != nil {
		return err
	}

	// Persist output resources and computed values. Secret values are never persisted on the owner
	// resource: they are materialized into a managed Radius.Security/secrets resource below.
	if err := resource.ApplyDeploymentOutput(rpv1.DeploymentOutput{DeployedOutputResources: outputResources, ComputedValues: computedValues}); err != nil {
		return err
	}

	// Fetch the resource type schema once and use it for both computed-value filtering and secret routing.
	schema, err := getResourceSchema(ctx, options.UcpClient, resource)
	if err != nil {
		return err
	}

	addComputedValuesToResourceProperties(resource, schema, computedValues)

	return d.materializeRecipeSecrets(ctx, resource, schema, secretValues)
}

// getResourceSchema fetches and returns the OpenAPI schema for the resource's type and API version.
func getResourceSchema(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resource *datamodel.DynamicResource) (map[string]any, error) {
	raw, err := GetSchemaForResourceType(ctx, ucpClient, resource.ID, resource.InternalMetadata.UpdatedAPIVersion)
	if err != nil {
		return nil, err
	}

	schema, ok := raw.(map[string]any)
	if !ok {
		return nil, ErrNoSchemaFound
	}
	return schema, nil
}

// addComputedValuesToResourceProperties copies recipe computed values onto the resource properties when the
// property name is declared in the schema and is not a framework-owned basic property. This avoids
// overwriting properties like application, environment, secrets and the secret reference.
func addComputedValuesToResourceProperties(resource *datamodel.DynamicResource, schema map[string]any, computedValues map[string]any) {
	resourceProps := schemaPropertyNames(schema)
	for key, value := range computedValues {
		if slices.Contains(resourceProps, key) {
			if resource.Properties == nil {
				resource.Properties = map[string]any{}
			}
			resource.Properties[key] = value
		}
	}
}

// materializeRecipeSecrets routes the recipe's secret outputs into a managed Radius.Security/secrets
// resource, and exposes a read-only reference to that resource on the owner via the reserved `secrets.name`
// sub-property. Secret values are never written onto the owner resource.
//
// A resource type opts into secret materialization by declaring a `secrets` block in its schema; that block
// is also what makes `secrets.name` a referenceable read-only property. Once a type opts in, ALL of the
// recipe's secret outputs are materialized — not just those whose keys are declared in the block. The
// declared keys document the type's expected secret surface but do not filter what is materialized, so a
// recipe (for example a direct AVM module) that emits an additional sensitive output does not silently lose
// it. When the resource type declares no `secrets` block, recipe secret outputs are dropped rather than
// persisted.
func (d *DynamicProcessor) materializeRecipeSecrets(ctx context.Context, resource *datamodel.DynamicResource, schema map[string]any, secretValues map[string]rpv1.SecretValueReference) error {
	if _, ok := schemautil.GetSecretsBlock(schema); !ok {
		return nil
	}

	data := make(map[string]string, len(secretValues))
	for key, ref := range secretValues {
		data[key] = ref.Value
	}
	if len(data) == 0 {
		return nil
	}

	if d.SecretMaterializer == nil {
		// No materializer configured (for example in unit tests); skip materialization.
		return nil
	}

	result, err := d.SecretMaterializer.Materialize(ctx, secret.Request{
		OwnerResourceID: resource.ID,
		EnvironmentID:   resource.ResourceMetadata().EnvironmentID(),
		ApplicationID:   resource.ResourceMetadata().ApplicationID(),
		Data:            data,
	})
	if err != nil {
		return err
	}

	// Expose only the managed secret's name via the reserved `secrets.name` reference. The secret data
	// keys declared in the block are never populated on the owner. Merge into any existing `secrets`
	// object so we don't clobber a declared block.
	if resource.Properties == nil {
		resource.Properties = map[string]any{}
	}
	secrets, ok := resource.Properties[schemautil.SecretsBlockPropertyName].(map[string]any)
	if !ok {
		secrets = map[string]any{}
		resource.Properties[schemautil.SecretsBlockPropertyName] = secrets
	}
	secrets[schemautil.SecretNameReferenceKey] = result.Name

	return nil
}

// schemaPropertyNames returns the property names declared in the schema, excluding framework-owned basic
// properties.
func schemaPropertyNames(schema map[string]any) []string {
	names := []string{}
	if properties, ok := schema["properties"].(map[string]any); ok {
		for key := range properties {
			if !slices.Contains(resourceutil.BasicProperties, key) {
				names = append(names, key)
			}
		}
	}
	return names
}

// GetSchemaForResourceType fetches the schema for a resource type from UCP
func GetSchemaForResourceType(ctx context.Context, ucp *v20231001preview.ClientFactory, resourceID string, apiVersion string) (any, error) {
	// Parse resourceID to get components
	ID, err := resources.Parse(resourceID)
	if err != nil {
		return nil, err
	}

	plane := ID.PlaneNamespace()
	planeName := strings.Split(plane, "/")[1]
	resourceProvider := strings.Split(ID.ProviderNamespace(), "/")[0]
	resourceType := strings.Split(ID.Type(), "/")[1]

	response, err := ucp.NewAPIVersionsClient().Get(
		ctx,
		planeName,
		resourceProvider,
		resourceType,
		apiVersion,
		nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoSchemaFound, err)
	}

	if response.Properties == nil || response.Properties.Schema == nil {
		return nil, ErrNoSchemaFound
	}

	return response.Properties.Schema, nil
}
