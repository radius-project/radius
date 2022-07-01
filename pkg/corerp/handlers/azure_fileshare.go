// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
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

func (handler *azureFileShareHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	armhandler := NewARMHandler(handler.arm)
	err := armhandler.Put(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	return nil
}

func (handler *azureFileShareHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	return resource.Identity, nil
}

func (handler *azureFileShareHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareIDKey, FileShareNameKey)
	if err != nil {
		return nil, err
	}

	return properties, nil
}
