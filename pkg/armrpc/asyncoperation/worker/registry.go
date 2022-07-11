// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"
	"sync"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
)

type ControllerFactoryFunc func(opts ctrl.Options) (ctrl.Controller, error)

// ControllerRegistry is an registry to register async controllers.
type ControllerRegistry struct {
	ctrlMap   map[string]ctrl.Controller
	ctrlMapMu sync.RWMutex
	sp        dataprovider.DataStorageProvider
}

// NewControllerRegistry creates an ControllerRegistry instance.
func NewControllerRegistry(sp dataprovider.DataStorageProvider) *ControllerRegistry {
	return &ControllerRegistry{
		ctrlMap: map[string]ctrl.Controller{},
		sp:      sp,
	}
}

// Register registers controller.
func (h *ControllerRegistry) Register(ctx context.Context, resourceType string, method v1.OperationMethod, factoryFn ControllerFactoryFunc, opts ctrl.Options) error {
	h.ctrlMapMu.Lock()
	defer h.ctrlMapMu.Unlock()

	ot := v1.OperationType{Type: resourceType, Method: method}

	storageClient, err := opts.DataProvider.GetStorageClient(ctx, resourceType)
	if err != nil {
		return err
	}
	opts.StorageClient = storageClient
	opts.ResourceType = resourceType

	ctrl, err := factoryFn(opts)
	if err != nil {
		return err
	}

	h.ctrlMap[ot.String()] = ctrl
	return nil
}

// Get gets the registered async controller instance.
func (h *ControllerRegistry) Get(operationType v1.OperationType) ctrl.Controller {
	h.ctrlMapMu.RLock()
	defer h.ctrlMapMu.RUnlock()

	if h, ok := h.ctrlMap[operationType.String()]; ok {
		return h
	}

	return nil
}
