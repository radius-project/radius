// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
)

const (
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
)

// NewAzureUserAssignedManagedIdentityHandler initializes a new handler for resources of kind UserAssignedManagedIdentity
func NewAzureUserAssignedManagedIdentityHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {

	// if !options.Resource.Deployed {
	// 	// TODO: right now this resource is already deployed during the rendering process :(
	// 	// this should be done here instead when we have built a more mature system.
	// }

	return map[string]string{}, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}

func NewAzureUserAssignedManagedIdentityHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureUserAssignedManagedIdentityHealthHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
