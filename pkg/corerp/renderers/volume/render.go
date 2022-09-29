// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	VolumeKindEphemeral               = "ephemeral"
	VolumeKindPersistent              = "persistent"
	PersistentVolumeKindAzureKeyVault = "azure.com.keyvault"
)

var storageAccountDependency outputresource.Dependency

// RendererType defines the type for the renderer function
type RendererType func(context.Context, *armauth.ArmConfig, conv.DataModelInterface, *renderers.RenderOptions) (renderers.RendererOutput, error)

type volumeFuncs struct {
	renderer RendererType
	secrets  func(name string) (map[string]rp.ComputedValueReference, map[string]rp.SecretValueReference)
}

var supportedVolumes = map[string]volumeFuncs{
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
func MakeSecretsAndValues(kind string, name string) (map[string]rp.ComputedValueReference, map[string]rp.SecretValueReference) {
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

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.VolumeResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties
	if properties.Kind == "" {
		return renderers.RendererOutput{}, errors.New("`kind` property is required")
	} else if !isSupported(properties.Kind) {
		return renderers.RendererOutput{}, fmt.Errorf("%v is not supported. Supported kind values: %v", properties.Kind, GetSupportedKinds())
	}

	renderOutput, err := r.VolumeRenderers[properties.Kind](ctx, r.Arm, dm, &options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues, secretValues := MakeSecretsAndValues(properties.Kind, storageAccountDependency.LocalID)

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
