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
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/curp/resources"
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
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := KeyVaultComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, errors.New("the 'resource' field cannot be specified when 'managed=true'")
		}

		// generate data we can use to manage a cosmosdb instance
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindAzureKeyVault,
			Resource: map[string]string{
				handlers.ManagedKey: "true",
			},
		}

		// It's already in the correct format
		return []workloads.WorkloadResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, errors.New("the 'resource' field is required when 'managed' is not specified")
		}

		vaultID, err := resources.Parse(component.Config.Resource)
		if err != nil {
			return nil, errors.New("the 'resource' field must be a valid resource id.")
		}

		err = vaultID.ValidateResourceType(KeyVaultResourceType)
		if err != nil {
			return nil, fmt.Errorf("the 'resource' field must refer to a KeyVault")
		}

		// generate data we can use to connect to a servicebus queue
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindAzureKeyVault,
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				handlers.KeyVaultIDKey:   vaultID.ID,
				handlers.KeyVaultNameKey: vaultID.Types[0].Name,
			},
		}

		// It's already in the correct format
		return []workloads.WorkloadResource{resource}, nil
	}
}
