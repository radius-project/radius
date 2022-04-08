// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

const (
	VolumeKindEphemeral                = "ephemeral"
	VolumeKindPersistent               = "persistent"
	PersistentVolumeKindAzureFileShare = "azure.com.fileshare"
	PersistentVolumeKindAzureKeyVault  = "azure.com.keyvault"
)

var storageAccountDependency outputresource.Dependency

// RendererType defines the type for the renderer function
type RendererType func(ctx context.Context, arm *armauth.ArmConfig, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error)
type volumeFuncs struct {
	renderer RendererType
	secrets  func(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference)
}

var supportedVolumes = map[string]volumeFuncs{
	PersistentVolumeKindAzureFileShare: {
		renderer: GetAzureFileShareVolume,
		secrets:  MakeSecretsAndValuesForAzureFileShare,
	},
	PersistentVolumeKindAzureKeyVault: {
		renderer: GetAzureKeyVaultVolume,
		secrets:  nil,
	},
}

// GetSupportedRenderers returns the list of supported volume types and corresponding Azure renderer
func GetSupportedRenderers() map[string]RendererType {
	result := map[string]RendererType{}
	for k, v := range supportedVolumes {
		result[k] = v.renderer
	}
	return result
}

// MakeSecretsAndValues invokes the secrets routine for the specified resource kind
func MakeSecretsAndValues(kind string, name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	volumeFuncs := supportedVolumes[kind]
	if volumeFuncs.secrets != nil {
		return volumeFuncs.secrets(name)
	}
	return nil, nil
}

// GetSupportedKinds returns a list of supported volume kinds
func GetSupportedKinds() []string {
	keys := []string{}
	for k := range supportedVolumes {
		keys = append(keys, k)
	}
	return keys
}

type Renderer struct {
	Arm             *armauth.ArmConfig
	VolumeRenderers map[string]RendererType
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.VolumeProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if properties.Kind == nil {
		return renderers.RendererOutput{}, errors.New("`kind` property is required")
	} else if !isSupported(*properties.Kind) {
		return renderers.RendererOutput{}, fmt.Errorf("%v is not supported. Supported kind values: %v", properties.Kind, GetSupportedKinds())
	}

	renderOutput, err := r.VolumeRenderers[*properties.Kind](ctx, r.Arm, options.Resource, options.Dependencies)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues, secretValues := MakeSecretsAndValues(*properties.Kind, storageAccountDependency.LocalID)

	return renderers.RendererOutput{
		Resources:      renderOutput.Resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func isSupported(kind string) bool {
	for _, k := range GetSupportedKinds() {
		if kind == k {
			return true
		}
	}
	return false
}
