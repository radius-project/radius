// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha3

import (
	"context"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Arm armauth.ArmConfig
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.AzureKeyVaultComponentProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := renderers.RendererOutput{}

	if properties.Managed != nil && *properties.Managed {
		if properties.Resource != nil && *properties.Resource != "" {
			return renderers.RendererOutput{}, renderers.ErrResourceSpecifiedForManagedResource
		}

		resource := outputresource.OutputResource{
			Resource: map[string]string{
				handlers.ManagedKey: "true",
			},
			Deployed:     false,
			LocalID:      outputresource.LocalIDKeyVault,
			Managed:      true,
			ResourceKind: resourcekinds.AzureKeyVault,
		}

		output.Resources = append(output.Resources, resource)
	} else {
		if properties.Resource == nil || *properties.Resource == "" {
			return renderers.RendererOutput{}, renderers.ErrResourceMissingForUnmanagedResource
		}

		vaultID, err := renderers.ValidateResourceID(*properties.Resource, KeyVaultResourceType, outputresource.LocalIDKeyVault)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resource := outputresource.OutputResource{
			LocalID:      outputresource.LocalIDKeyVault,
			ResourceKind: resourcekinds.AzureKeyVault,
			Managed:      false,
			Deployed:     true,
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				handlers.KeyVaultIDKey:   vaultID.ID,
				handlers.KeyVaultNameKey: vaultID.Types[0].Name,
			},
		}

		output.Resources = append(output.Resources, resource)
	}

	output.ComputedValues = map[string]renderers.ComputedValueReference{
		"uri": {
			LocalID:           outputresource.LocalIDKeyVault,
			PropertyReference: handlers.KeyVaultURIKey,
		},
	}

	return output, nil
}
