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

type ControllerFactoryFunc func(store.StorageClient) (asyncctrl.AsyncControllerInterface, error)

type HandlerRegistry struct {
	ctrlMap   map[string]asyncctrl.AsyncControllerInterface
	ctrlMapMu sync.Mutex
	sp        dataprovider.DataStorageProvider
}

func NewHandlerRegistry(sp dataprovider.DataStorageProvider) *HandlerRegistry {
	return &HandlerRegistry{
		ctrlMap: map[string]asyncctrl.AsyncControllerInterface{},
		sp:      sp,
	}
}

func (h *HandlerRegistry) RegisterController(ctx context.Context, operationName, resourceTypeName string, factoryFn ControllerFactoryFunc) error {
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

func (h *HandlerRegistry) GetController(name string) asyncctrl.AsyncControllerInterface {
	h.ctrlMapMu.Lock()
	defer h.ctrlMapMu.Unlock()

	if h, ok := h.ctrlMap[name]; ok {
		return h
	}

	return nil
}
