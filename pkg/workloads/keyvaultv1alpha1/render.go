// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2019-09-01/keyvault"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/components"
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
	vaultName := properties[KeyVaultName]
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
				URIProperty: *vault.Properties.VaultURI,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for keyvault workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := KeyVaultComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a keyvault instance
	resource := workloads.WorkloadResource{
		Type: "azure.keyvault",
		Resource: map[string]string{
			"name": w.Workload.Name,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}
