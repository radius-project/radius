// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
)

const (
	FileShareNameKey = "fileshare"
	FileShareIDKey   = "fileshareid"
)

func NewAzureFileShareHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureFileShareHandler{arm: arm}
}

type azureFileShareHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureFileShareHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareIDKey)
	if err != nil {
		return nil, err
	}

	armhandler := NewARMHandler(handler.arm)
	properties, err = armhandler.Put(ctx, options)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, options DeleteOptions) error {
	return nil
}

func NewAzureFileShareHealthHandler(arm *armauth.ArmConfig) HealthHandler {
	return &azureFileShareHealthHandler{arm: arm}
}

type azureFileShareHealthHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureFileShareHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
