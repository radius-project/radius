// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
)

// NewAzureUserAssignedManagedIdentityHandler initializes a new handler for resources of kind UserAssignedManagedIdentity
func NewAzureUserAssignedManagedIdentityHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm armauth.ArmConfig
}

func (pih *azureUserAssignedManagedIdentityHandler) GetProperties(resource workloads.OutputResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	p := map[string]string{
		"kind":       item.GetKind(),
		"apiVersion": item.GetAPIVersion(),
		"namespace":  item.GetNamespace(),
		"name":       item.GetName(),
	}
	return p, nil
}

func (pih *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	if options.Resource.Created {
		// TODO: right now this resource is created during the rendering process :(
		// this should be done here instead when we have built a more mature system.
	}
	return properties, nil
}

func (pih *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}
