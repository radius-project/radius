// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"runtime"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/ucp/util"
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
	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareIDKey, FileShareNameKey)
	if err != nil {
		return nil, err
	}

	armhandler := NewARMHandler(handler.arm)
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("@@@@@@ Before calling armhandler.Put in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
	properties, err = armhandler.Put(ctx, options)
	if err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("@@@@@@ After calling armhandler.Put in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
	return properties, nil
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
