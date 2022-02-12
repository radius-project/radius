// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha3

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Arm armauth.ArmConfig
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.AzureKeyVaultProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := renderers.RendererOutput{}

	if properties.Resource == nil || *properties.Resource == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForResource
	}

	vaultID, err := renderers.ValidateResourceID(*properties.Resource, KeyVaultResourceType, outputresource.LocalIDKeyVault)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDKeyVault,
		ResourceKind: resourcekinds.AzureKeyVault,
		Deployed:     true,
		Resource: map[string]string{
			handlers.KeyVaultIDKey:   vaultID.ID,
			handlers.KeyVaultNameKey: vaultID.Types[0].Name,
		},
	}

	output.Resources = append(output.Resources, resource)

	output.ComputedValues = map[string]renderers.ComputedValueReference{
		"uri": {
			LocalID:           outputresource.LocalIDKeyVault,
			PropertyReference: handlers.KeyVaultURIKey,
		},
	}

	return output, nil
}
