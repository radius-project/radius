// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	"github.com/Azure/go-autorest/autorest/to"
)

const (
	EnvironmentComputeKindKubernetes = "kubernetes"
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
		Properties: datamodel.EnvironmentProperties{
			UseDevRecipes: to.Bool(src.Properties.UseDevRecipes),
		},
	}

	envCompute, err := toEnvironmentComputeDataModel(src.Properties.Compute)
	if err != nil {
		return nil, err
	}
	converted.Properties.Compute = *envCompute

	if src.Properties.Recipes != nil {
		recipes := make(map[string]datamodel.EnvironmentRecipeProperties)
		for key, val := range src.Properties.Recipes {
			if val != nil {
				recipes[key] = datamodel.EnvironmentRecipeProperties{
					LinkType:     to.String(val.LinkType),
					TemplatePath: to.String(val.TemplatePath),
					Parameters:   val.Parameters,
				}
			}
		}

		converted.Properties.Recipes = recipes
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

	dst.ID = to.StringPtr(env.ID)
	dst.Name = to.StringPtr(env.Name)
	dst.Type = to.StringPtr(env.Type)
	dst.SystemData = fromSystemDataModel(env.SystemData)
	dst.Location = to.StringPtr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.InternalMetadata.AsyncProvisioningState),
		UseDevRecipes:     to.BoolPtr(env.Properties.UseDevRecipes),
	}

	dst.Properties.Compute = fromEnvironmentComputeDataModel(&env.Properties.Compute)
	if dst.Properties.Compute == nil {
		return v1.ErrInvalidModelConversion
	}

	if env.Properties.Recipes != nil {
		recipes := make(map[string]*EnvironmentRecipeProperties)
		for key, val := range env.Properties.Recipes {
			recipes[key] = &EnvironmentRecipeProperties{
				LinkType:     to.StringPtr(val.LinkType),
				TemplatePath: to.StringPtr(val.TemplatePath),
				Parameters:   val.Parameters,
			}
		}
		dst.Properties.Recipes = recipes
	}

	if env.Properties.Providers != (datamodel.Providers{}) {
		dst.Properties.Providers = &Providers{}
		if env.Properties.Providers.Azure != (datamodel.ProvidersAzure{}) {
			dst.Properties.Providers.Azure = &ProvidersAzure{
				Scope: to.StringPtr(env.Properties.Providers.Azure.Scope),
			}
		}
		if env.Properties.Providers.AWS != (datamodel.ProvidersAWS{}) {
			dst.Properties.Providers.Aws = &ProvidersAws{
				Scope: to.StringPtr(env.Properties.Providers.AWS.Scope),
			}
		}
	}

	var extensions []EnvironmentExtensionClassification
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
				Kind:       toIdentityKind(v.Identity.Kind),
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
			Namespace: to.StringPtr(envCompute.KubernetesCompute.Namespace),
			Identity:  identity,
		}
		if envCompute.KubernetesCompute.ResourceID != "" {
			compute.ResourceID = to.StringPtr(envCompute.KubernetesCompute.ResourceID)
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
		k = EnvironmentComputeKindKubernetes // 2022-03-15-privatepreview supports only kubernetes.
	}

	return &k
}

// fromExtensionClassificationEnvDataModel: Converts from base datamodel to versioned datamodel
func fromEnvExtensionClassificationDataModel(e datamodel.Extension) EnvironmentExtensionClassification {
	switch e.Kind {
	case datamodel.KubernetesMetadata:
		var ann, lbl = fromExtensionClassificationFields(e)
		return &EnvironmentKubernetesMetadataExtension{
			Kind:        to.StringPtr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}
	}

	return nil
}

// toEnvExtensionDataModel: Converts from versioned datamodel to base datamodel
func toEnvExtensionDataModel(e EnvironmentExtensionClassification) datamodel.Extension {
	switch c := e.(type) {
	case *EnvironmentKubernetesMetadataExtension:
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
