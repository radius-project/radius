// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2019-09-01/keyvault"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the keyvault workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for keyvault workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != outputresource.KindAzureKeyVault {
		return nil, fmt.Errorf("cannot fulfill binding - expected properties for %s", outputresource.KindAzureKeyVault)
	}

	properties := resources[0].Properties
	vaultName := properties[handlers.KeyVaultNameKey]
	kvClient := azclients.NewVaultsClient(r.Arm.SubscriptionID, r.Arm.Auth)
	vault, err := kvClient.Get(ctx, r.Arm.ResourceGroup, vaultName)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch keyvault")
	}

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Kind:      "azure.com/KeyVault",
			Properties: map[string]interface{}{
				handlers.KeyVaultURIKey: *vault.Properties.VaultURI,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for keyvault workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := KeyVaultComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	var resource outputresource.OutputResource
	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		resource = outputresource.OutputResource{
			LocalID:  outputresource.LocalIDKeyVault,
			Kind:     outputresource.KindAzureKeyVault,
			Type:     outputresource.TypeARM,
			Managed:  true,
			Deployed: false,
			Resource: map[string]string{
				handlers.ManagedKey: "true",
			},
		}
	} else {
		if component.Config.Resource == "" {
			return nil, workloads.ErrResourceMissingForUnmanagedResource
		}

		vaultID, err := workloads.ValidateResourceID(component.Config.Resource, KeyVaultResourceType, outputresource.LocalIDKeyVault)
		if err != nil {
			return nil, err
		}

		resource = outputresource.OutputResource{
			LocalID:  outputresource.LocalIDKeyVault,
			Kind:     outputresource.KindAzureKeyVault,
			Managed:  false,
			Deployed: true,
			Type:     outputresource.TypeARM,
			Info: outputresource.ARMInfo{
				ID:           vaultID.ID,
				ResourceType: KeyVaultResourceType.Type(),
				APIVersion:   keyvault.Version(),
			},
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				handlers.KeyVaultIDKey:   vaultID.ID,
				handlers.KeyVaultNameKey: vaultID.Types[0].Name,
			},
		}
	}

	return []outputresource.OutputResource{resource}, nil
}
