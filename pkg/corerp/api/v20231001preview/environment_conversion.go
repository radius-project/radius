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

package v20231001preview

import (
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubernetes"
	types "github.com/radius-project/radius/pkg/recipes"

	rp_util "github.com/radius-project/radius/pkg/rp/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

const (
	EnvironmentComputeKindKubernetes = "kubernetes"
	invalidLocalModulePathFmt        = "local module paths are not supported with Terraform Recipes. The 'templatePath' '%s' was detected as a local module path because it begins with '/' or './' or '../'."
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *EnvironmentResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	converted := &datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.EnvironmentProperties{},
	}

	envCompute, err := toEnvironmentComputeDataModel(src.Properties.Compute)
	if err != nil {
		return nil, err
	}
	converted.Properties.Compute = *envCompute

	if src.Properties.Recipes != nil {
		envRecipes := make(map[string]map[string]datamodel.EnvironmentRecipeProperties)
		for resourceType, recipes := range src.Properties.Recipes {
			if !rp_util.IsValidPortableResourceType(resourceType) {
				return &datamodel.Environment{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid resource type: %q", resourceType))
			}
			envRecipes[resourceType] = map[string]datamodel.EnvironmentRecipeProperties{}
			for recipeName, recipeDetails := range recipes {
				if recipeDetails != nil {
					if recipeDetails.GetRecipeProperties().TemplateKind == nil || !isValidTemplateKind(*recipeDetails.GetRecipeProperties().TemplateKind) {
						formats := []string{}
						for _, format := range types.SupportedTemplateKind {
							formats = append(formats, fmt.Sprintf("%q", format))
						}
						return &datamodel.Environment{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid template kind. Allowed formats: %s", strings.Join(formats, ", ")))
					}
					envRecipes[resourceType][recipeName], err = toEnvironmentRecipeProperties(recipeDetails)
					if err != nil {
						return &datamodel.Environment{}, err
					}
				}
			}

		}
		converted.Properties.Recipes = envRecipes
	}

	if src.Properties.Providers != nil {
		if src.Properties.Providers.Azure != nil {
			converted.Properties.Providers.Azure = datamodel.ProvidersAzure{
				Scope: to.String(src.Properties.Providers.Azure.Scope),
			}
		}
		if src.Properties.Providers.Aws != nil {
			converted.Properties.Providers.AWS = datamodel.ProvidersAWS{
				Scope: to.String(src.Properties.Providers.Aws.Scope),
			}
		}
	}

	if src.Properties.Simulated != nil && *src.Properties.Simulated {
		converted.Properties.Simulated = true
	}

	var extensions []datamodel.Extension
	if src.Properties.Extensions != nil {
		for _, e := range src.Properties.Extensions {
			extensions = append(extensions, toEnvExtensionDataModel(e))
		}
		converted.Properties.Extensions = extensions
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *EnvironmentResource) ConvertFrom(src v1.DataModelInterface) error {
	env, ok := src.(*datamodel.Environment)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(env.ID)
	dst.Name = to.Ptr(env.Name)
	dst.Type = to.Ptr(env.Type)
	dst.SystemData = fromSystemDataModel(env.SystemData)
	dst.Location = to.Ptr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.InternalMetadata.AsyncProvisioningState),
	}

	dst.Properties.Compute = fromEnvironmentComputeDataModel(&env.Properties.Compute)
	if dst.Properties.Compute == nil {
		return v1.ErrInvalidModelConversion
	}

	if env.Properties.Recipes != nil {
		recipes := make(map[string]map[string]RecipePropertiesClassification)
		for resourceType, recipe := range env.Properties.Recipes {
			recipes[resourceType] = map[string]RecipePropertiesClassification{}
			for recipeName, recipeDetails := range recipe {
				recipes[resourceType][recipeName] = fromRecipePropertiesClassificationDatamodel(recipeDetails)
			}
		}
		dst.Properties.Recipes = recipes
	}

	if env.Properties.Providers != (datamodel.Providers{}) {
		dst.Properties.Providers = &Providers{}
		if env.Properties.Providers.Azure != (datamodel.ProvidersAzure{}) {
			dst.Properties.Providers.Azure = &ProvidersAzure{
				Scope: to.Ptr(env.Properties.Providers.Azure.Scope),
			}
		}
		if env.Properties.Providers.AWS != (datamodel.ProvidersAWS{}) {
			dst.Properties.Providers.Aws = &ProvidersAws{
				Scope: to.Ptr(env.Properties.Providers.AWS.Scope),
			}
		}
	}

	if env.Properties.Simulated {
		dst.Properties.Simulated = to.Ptr(env.Properties.Simulated)
	}

	var extensions []ExtensionClassification
	if env.Properties.Extensions != nil {
		for _, e := range env.Properties.Extensions {
			extensions = append(extensions, fromEnvExtensionClassificationDataModel(e))
		}
		dst.Properties.Extensions = extensions
	}

	return nil
}

func toEnvironmentComputeDataModel(h EnvironmentComputeClassification) (*rpv1.EnvironmentCompute, error) {
	switch v := h.(type) {
	case *KubernetesCompute:
		k, err := toEnvironmentComputeKindDataModel(*v.Kind)
		if err != nil {
			return nil, err
		}

		if !kubernetes.IsValidObjectName(to.String(v.Namespace)) {
			return nil, &v1.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"}
		}

		var identity *rpv1.IdentitySettings
		if v.Identity != nil {
			identity = &rpv1.IdentitySettings{
				Kind:       toIdentityKindDataModel(v.Identity.Kind),
				Resource:   to.String(v.Identity.Resource),
				OIDCIssuer: to.String(v.Identity.OidcIssuer),
			}
		}

		return &rpv1.EnvironmentCompute{
			Kind: k,
			KubernetesCompute: rpv1.KubernetesComputeProperties{
				ResourceID: to.String(v.ResourceID),
				Namespace:  to.String(v.Namespace),
			},
			Identity: identity,
		}, nil
	default:
		return nil, v1.ErrInvalidModelConversion
	}
}

func fromEnvironmentComputeDataModel(envCompute *rpv1.EnvironmentCompute) EnvironmentComputeClassification {
	if envCompute == nil {
		return nil
	}

	switch envCompute.Kind {
	case rpv1.KubernetesComputeKind:
		var identity *IdentitySettings
		if envCompute.Identity != nil {
			identity = &IdentitySettings{
				Kind:       fromIdentityKind(envCompute.Identity.Kind),
				Resource:   toStringPtr(envCompute.Identity.Resource),
				OidcIssuer: toStringPtr(envCompute.Identity.OIDCIssuer),
			}
		}
		compute := &KubernetesCompute{
			Kind:      fromEnvironmentComputeKind(envCompute.Kind),
			Namespace: to.Ptr(envCompute.KubernetesCompute.Namespace),
			Identity:  identity,
		}
		if envCompute.KubernetesCompute.ResourceID != "" {
			compute.ResourceID = to.Ptr(envCompute.KubernetesCompute.ResourceID)
		}
		return compute
	default:
		return nil
	}
}

func toEnvironmentComputeKindDataModel(kind string) (rpv1.EnvironmentComputeKind, error) {
	switch kind {
	case EnvironmentComputeKindKubernetes:
		return rpv1.KubernetesComputeKind, nil
	default:
		return rpv1.UnknownComputeKind, &v1.ErrModelConversion{PropertyName: "$.properties.compute.kind", ValidValue: "[kubernetes]"}
	}
}

func fromEnvironmentComputeKind(kind rpv1.EnvironmentComputeKind) *string {
	var k string
	switch kind {
	case rpv1.KubernetesComputeKind:
		k = EnvironmentComputeKindKubernetes
	default:
		k = EnvironmentComputeKindKubernetes // 2023-10-01-preview supports only kubernetes.
	}

	return &k
}

// fromExtensionClassificationEnvDataModel: Converts from base datamodel to versioned datamodel
func fromEnvExtensionClassificationDataModel(e datamodel.Extension) ExtensionClassification {
	switch e.Kind {
	case datamodel.KubernetesMetadata:
		var ann, lbl = fromExtensionClassificationFields(e)
		return &KubernetesMetadataExtension{
			Kind:        to.Ptr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}
	}

	return nil
}

// toEnvExtensionDataModel: Converts from versioned datamodel to base datamodel
func toEnvExtensionDataModel(e ExtensionClassification) datamodel.Extension {
	switch c := e.(type) {
	case *KubernetesMetadataExtension:
		return datamodel.Extension{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: to.StringMap(c.Annotations),
				Labels:      to.StringMap(c.Labels),
			},
		}
	}

	return datamodel.Extension{}
}

func toEnvironmentRecipeProperties(e RecipePropertiesClassification) (datamodel.EnvironmentRecipeProperties, error) {
	switch c := e.(type) {
	case *TerraformRecipeProperties:
		if c.TemplatePath != nil {
			// Check for local paths
			if strings.HasPrefix(to.String(c.TemplatePath), "/") || strings.HasPrefix(to.String(c.TemplatePath), "./") || strings.HasPrefix(to.String(c.TemplatePath), "../") {
				return datamodel.EnvironmentRecipeProperties{}, v1.NewClientErrInvalidRequest(fmt.Sprintf(invalidLocalModulePathFmt, to.String(c.TemplatePath)))
			}
		}
		return datamodel.EnvironmentRecipeProperties{
			TemplateKind:    types.TemplateKindTerraform,
			TemplateVersion: to.String(c.TemplateVersion),
			TemplatePath:    to.String(c.TemplatePath),
			Parameters:      c.Parameters,
		}, nil
	case *BicepRecipeProperties:
		return datamodel.EnvironmentRecipeProperties{
			TemplateKind: types.TemplateKindBicep,
			TemplatePath: to.String(c.TemplatePath),
			Parameters:   c.Parameters,
		}, nil
	}
	return datamodel.EnvironmentRecipeProperties{}, nil
}

func fromRecipePropertiesClassificationDatamodel(e datamodel.EnvironmentRecipeProperties) RecipePropertiesClassification {
	switch e.TemplateKind {
	case types.TemplateKindTerraform:
		return &TerraformRecipeProperties{
			TemplateKind:    to.Ptr(e.TemplateKind),
			TemplateVersion: to.Ptr(e.TemplateVersion),
			TemplatePath:    to.Ptr(e.TemplatePath),
			Parameters:      e.Parameters,
		}
	case types.TemplateKindBicep:
		return &BicepRecipeProperties{
			TemplateKind: to.Ptr(e.TemplateKind),
			TemplatePath: to.Ptr(e.TemplatePath),
			Parameters:   e.Parameters,
		}
	}
	return nil
}
