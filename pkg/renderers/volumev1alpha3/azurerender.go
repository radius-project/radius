// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
)

const (
	VolumeKindEphemeral  = "ephemeral"
	VolumeKindPersistent = "persistent"
)

var storageAccountDependency outputresource.Dependency

type AzureRenderer struct {
	Arm             armauth.ArmConfig
	VolumeRenderers map[string]func(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error)
}

var SupportedVolumeRenderers = map[string]func(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error){
	"azure.com.fileshare": GetAzureFileShareVolume,
}

var SupportedVolumeMakeSecretsAndValues = map[string]func(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference){
	"azure.com.fileshare": MakeSecretsAndValuesForAzureFileShare,
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r *AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := VolumeProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if !isSupported(properties.Kind) {
		return renderers.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %v", properties.Kind, SupportedVolumeRenderers)
	}

	renderOutput, err := r.VolumeRenderers[properties.Kind](ctx, options.Resource, options.Dependencies)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues, secretValues := SupportedVolumeMakeSecretsAndValues[properties.Kind](storageAccountDependency.LocalID)

	return renderers.RendererOutput{
		Resources:      renderOutput.Resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func isSupported(kind string) bool {
	for k := range SupportedVolumeRenderers {
		if kind == k {
			return true
		}
	}
	return false
}
