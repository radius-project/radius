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
	KeyVaultSecretNameKey  = "keyvaultsecretname"
	KeyVaultSecretValueKey = "keyvaultsecretvalue"
)

// NewAzureKeyVaultSecretHandler initializes a new handler for resources of kind Azure KeyVault Secret
func NewAzureKeyVaultSecretHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureKeyVaultSecretHandler{arm: arm}
}

type azureKeyVaultSecretHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureKeyVaultSecretHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {

	// if !options.Resource.Deployed {
	// 	// TODO: right now this resource is already deployed during the rendering process :(
	// 	// this should be done here instead when we have built a more mature system.
	// }

	return map[string]string{}, nil
}

func (handler *azureKeyVaultSecretHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}

func NewAzureKeyVaultSecretHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureKeyVaultSecretHealthHandler{arm: arm}
}

type azureKeyVaultSecretHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureKeyVaultSecretHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
