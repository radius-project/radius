// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2019-09-01/keyvault"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
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

	if len(resources) != 1 || resources[0].Type != workloads.ResourceKindAzureKeyVault {
		return nil, fmt.Errorf("cannot fulfill binding - expected properties for %s", workloads.ResourceKindAzureKeyVault)
	}

	properties := resources[0].Properties
	vaultName := properties[handlers.KeyVaultNameKey]
	kvClient := keyvault.NewVaultsClient(r.Arm.SubscriptionID)
	kvClient.Authorizer = r.Arm.Auth
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
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := KeyVaultComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		resource := workloads.OutputResource{
			LocalID:            "KeyVault",
			ResourceKind:       workloads.ResourceKindAzureKeyVault,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Deployed:           false,
			Resource: map[string]string{
				handlers.ManagedKey: "true",
			},
		}

		// It's already in the correct format
		return []workloads.OutputResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, workloads.ErrResourceMissingForUnmanagedResource
		}

		vaultID, err := workloads.ValidateResourceID(component.Config.Resource, KeyVaultResourceType, "KeyVault")
		if err != nil {
			return nil, err
		}

		resource := workloads.OutputResource{
			LocalID:            "KeyVault",
			ResourceKind:       workloads.ResourceKindAzureKeyVault,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Deployed:           true,
			Managed:            false,
			OutputResourceInfo: workloads.ARMInfo{
				ResourceID:   vaultID.ID,
				ResourceType: KeyVaultResourceType.Type(),
				APIVersion:   strings.Split(strings.Split(keyvault.UserAgent(), "keyvault/")[1], " profiles")[0],
			},
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				handlers.KeyVaultIDKey:   vaultID.ID,
				handlers.KeyVaultNameKey: vaultID.Types[0].Name,
			},
		}

		// It's already in the correct format
		return []workloads.OutputResource{resource}, nil
	}
}
