// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"sync"

	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/store"
)

type ControllerFactoryFunc func(store.StorageClient) (asyncctrl.AsyncController, error)

// ControllerRegistry is an registry to register async controllers.
type ControllerRegistry struct {
	ctrlMap   map[string]asyncctrl.AsyncController
	ctrlMapMu sync.RWMutex
	sp        dataprovider.DataStorageProvider
}

// NewControllerRegistry creates an ControllerRegistry instance.
func NewControllerRegistry(sp dataprovider.DataStorageProvider) *ControllerRegistry {
	return &ControllerRegistry{
		ctrlMap: map[string]asyncctrl.AsyncController{},
		sp:      sp,
	}
}

// Register registers controller.
func (h *ControllerRegistry) Register(ctx context.Context, operationName, resourceTypeName string, factoryFn ControllerFactoryFunc) error {
	h.ctrlMapMu.Lock()
	defer h.ctrlMapMu.Unlock()

	sc, err := h.sp.GetStorageClient(ctx, resourceTypeName)
	if err != nil {
		return err
	}

	ctrl, err := factoryFn(sc)
	if err != nil {
		return err
	}

	h.ctrlMap[operationName] = ctrl
	return nil
}

// Get gets the registered async controller instance.
func (h *ControllerRegistry) Get(operationName string) asyncctrl.AsyncController {
	h.ctrlMapMu.RLock()
	defer h.ctrlMapMu.RUnlock()

	if h, ok := h.ctrlMap[operationName]; ok {
		return h
	}

	return nil
}
