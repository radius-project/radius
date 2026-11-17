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

package recipes

import (
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// ValidateRecipeDefinition validates delegation metadata without choosing a target instance.
func ValidateRecipeDefinition(resourceType string, definition *datamodel.RecipeDefinition) error {
	if definition == nil {
		return fmt.Errorf("recipe for resource type %q is missing its definition", resourceType)
	}
	if definition.Kind == datamodel.RecipeKindDelegated {
		if definition.Source != "" || definition.Parameters != nil || definition.Outputs != nil || definition.SecretOutputs != nil || definition.PlainHTTP {
			return fmt.Errorf("delegated recipe for resource type %q cannot specify provisioning fields", resourceType)
		}
		if len(definition.AcceptsDelegationFrom) != 0 {
			return fmt.Errorf("delegated recipe cannot accept delegation")
		}
		if err := validateDelegationResourceType(resourceType); err != nil {
			return err
		}
		if definition.DelegatesTo == "" {
			return fmt.Errorf("delegated recipe requires delegatesTo")
		}
		if err := validateDelegationResourceType(definition.DelegatesTo); err != nil {
			return err
		}
		if strings.EqualFold(resourceType, definition.DelegatesTo) {
			return fmt.Errorf("a resource type cannot delegate provisioning to itself")
		}
	} else {
		if definition.Kind != TemplateKindBicep && definition.Kind != TemplateKindTerraform {
			return fmt.Errorf("recipe requires kind bicep, terraform, or delegated")
		}
		if strings.TrimSpace(definition.Source) == "" {
			return fmt.Errorf("a provisioning recipe requires a source")
		}
		if definition.DelegatesTo != "" {
			return fmt.Errorf("only a delegated recipe can specify delegatesTo")
		}
	}
	seen := make(map[string]struct{}, len(definition.AcceptsDelegationFrom))
	for _, accepted := range definition.AcceptsDelegationFrom {
		if err := validateDelegationResourceType(accepted); err != nil {
			return fmt.Errorf("invalid acceptsDelegationFrom entry: %w", err)
		}
		key := strings.ToLower(accepted)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate accepted resource type %q", accepted)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateDelegationResourceType(resourceType string) error {
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/validation/providers/" + resourceType + "/validation")
	if err != nil || id.Type() != resourceType {
		return fmt.Errorf("%q is not a fully qualified resource type", resourceType)
	}
	return nil
}

// SelectDelegationTarget resolves exactly one distinct annotated reference of the registered type.
// The caller validates target consent, scope, and lifecycle bindings.
func SelectDelegationTarget(properties, sourceSchema map[string]any, targetType string) (resources.ID, error) {
	if err := validateDelegationResourceType(targetType); err != nil {
		return resources.ID{}, err
	}
	references, err := schema.ExtractResourceReferences(properties, sourceSchema)
	if err != nil {
		return resources.ID{}, err
	}
	var selected resources.ID
	for _, reference := range references {
		if !strings.EqualFold(reference.Type, targetType) {
			continue
		}
		id, err := resources.ParseResource(reference.ID)
		if err != nil {
			return resources.ID{}, fmt.Errorf("invalid related target resource ID")
		}
		if !selected.IsEmpty() && !resources.IDEquals(selected, id) {
			return resources.ID{}, fmt.Errorf("delegation requires exactly one distinct target resource")
		}
		selected = id
	}
	if selected.IsEmpty() {
		return resources.ID{}, fmt.Errorf("delegation requires exactly one distinct target resource")
	}
	return selected, nil
}

// ValidateDelegationBinding checks consent and common ownership boundaries before saving a binding.
func ValidateDelegationBinding(source, target ResourceMetadata, targetRecipe *EnvironmentDefinition, previousTargetID string) error {
	sourceID, err := resources.ParseResource(source.ResourceID)
	if err != nil {
		return fmt.Errorf("invalid delegating Radius resource ID")
	}
	targetID, err := resources.ParseResource(target.ResourceID)
	if err != nil {
		return fmt.Errorf("invalid target Radius resource ID")
	}
	if sourceID.FindScope("radius") == "" || targetID.FindScope("radius") == "" ||
		!strings.EqualFold(sourceID.RootScope(), targetID.RootScope()) ||
		source.EnvironmentID == "" || !strings.EqualFold(source.EnvironmentID, target.EnvironmentID) ||
		!strings.EqualFold(source.ApplicationID, target.ApplicationID) {
		return fmt.Errorf("delegation requires the same tenant, plane, resource group, application, and environment")
	}
	if resources.IDEquals(sourceID, targetID) {
		return fmt.Errorf("a resource cannot delegate provisioning to itself")
	}
	if previousTargetID != "" && !strings.EqualFold(previousTargetID, target.ResourceID) {
		return fmt.Errorf("delete the delegating resource before recreating it against a different target")
	}
	if targetRecipe == nil || targetRecipe.DelegatesTo != "" ||
		(targetRecipe.Driver != TemplateKindBicep && targetRecipe.Driver != TemplateKindTerraform) {
		return fmt.Errorf("the delegation target must run a provisioning recipe")
	}
	for _, accepted := range targetRecipe.AcceptsDelegationFrom {
		if strings.EqualFold(accepted, sourceID.Type()) {
			return nil
		}
	}
	return fmt.Errorf("the target recipe does not accept delegation from resource type %q", sourceID.Type())
}
