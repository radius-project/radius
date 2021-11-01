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

var storageAccountDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Arm             armauth.ArmConfig
	VolumeRenderers map[string]func(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error)
}

var SupportedVolumeKinds = map[string]func(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error){
	"azure.com.fileshare": GetAzureFileShareVolume,
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := VolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if !isSupported(properties.Kind) {
		return renderers.RendererOutput{}, fmt.Errorf("%s is not supported. Supported kind values: %v", properties.Kind, SupportedVolumeKinds)
	}

	renderOutput, err := r.VolumeRenderers[properties.Kind](ctx, resource, dependencies)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues, secretValues := MakeSecretsAndValues(storageAccountDependency.LocalID)

	return renderers.RendererOutput{
		Resources:      renderOutput.Resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func isSupported(kind string) bool {
	for k, _ := range SupportedVolumeKinds {
		if kind == k {
			return true
		}
	}
	return false
}

func MakeSecretsAndValues(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		StorageAccountName: {
			LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
			Value:   name,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		StorageKeyValue: {
			LocalID: storageAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/storagerp/storage-accounts/list-keys
			Action:        "listKeys",
			ValueSelector: "/keys/0/value",
		},
	}

	return computedValues, secretValues
}
