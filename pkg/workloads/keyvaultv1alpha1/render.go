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

// Renderer is the WorkloadRenderer implementation for the service bus workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "azure.com/KeyVault" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != "azure.keyvault" {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for azure.keyvault")
	}

	properties := wrp[0].Properties
	vaultName := properties["keyvaultname"]
	msiResourceID := properties["keyvaultmsiresourceid"]
	kvClient := keyvault.NewVaultsClient(r.Arm.SubscriptionID)
	kvClient.Authorizer = r.Arm.Auth
	vault, err := kvClient.Get(ctx, r.Arm.ResourceGroup, vaultName)
	if err != nil {
		return nil, fmt.Errorf("Cannot fetch keyvault")
	}

	values := map[string]interface{}{
		"uri":   *vault.Properties.VaultURI,
		"msiId": msiResourceID,
	}

	return values, nil
}

// Render is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := ServiceBusQueueComponent{}
	err := components.ConvertFromGeneric(w.Workload, &component)
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
