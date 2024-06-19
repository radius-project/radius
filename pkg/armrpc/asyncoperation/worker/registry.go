/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package worker

import (
	"context"
	"sync"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
)

const (
	// ResourceTypeAny is a wildcard for any resource type.
	ResourceTypeAny = "*"

	// OperationMethodAny is a wildcard for any operation method.
	OperationMethodAny = "*"
)

// ControllerFactoryFunc is a factory function to create a controller.
type ControllerFactoryFunc func(opts ctrl.Options) (ctrl.Controller, error)

// ControllerRegistry is an registry to register async controllers.
type ControllerRegistry struct {
	ctrlMap   map[string]ctrl.Controller
	ctrlMapMu sync.RWMutex
	sp        dataprovider.DataStorageProvider

	// Fallback allows the registration of a controller that will be used
	// for operations that don't match any other operation type.
	fallbackFactory ControllerFactoryFunc
	fallbackOpts    ctrl.Options
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
	if resourceType == ResourceTypeAny && method == OperationMethodAny {
		// This is a fallback controller. Skip registration for now so we can create instances
		// dynamically when needed.
		h.fallbackFactory = factoryFn
		h.fallbackOpts = opts
		return nil
	}

	storageClient, err := h.sp.GetStorageClient(ctx, resourceType)
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
func (h *ControllerRegistry) Get(ctx context.Context, operationType v1.OperationType) (ctrl.Controller, error) {
	h.ctrlMapMu.RLock()
	defer h.ctrlMapMu.RUnlock()

	if h, ok := h.ctrlMap[operationType.String()]; ok {
		return h, nil
	}

	// If no controller is found, then look for a default controller.
	if h.fallbackFactory == nil {
		return nil, nil
	}

	storageClient, err := h.sp.GetStorageClient(ctx, operationType.Type)
	if err != nil {
		return nil, err
	}

	// Copy the options so we can update it.
	opts := h.fallbackOpts

	opts.StorageClient = storageClient
	opts.ResourceType = operationType.Type

	return h.fallbackFactory(opts)
}
