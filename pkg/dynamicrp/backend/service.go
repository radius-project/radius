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

package backend

import (
	"context"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
)

// Service runs the backend for the dynamic-rp.
type Service struct {
	worker.Service
	options *dynamicrp.Options
	recipes *controllerconfig.RecipeControllerConfig
}

// NewService creates a new service to run the dynamic-rp backend.
func NewService(options *dynamicrp.Options) *Service {
	workerOptions := worker.Options{}
	if options.Config.Worker.MaxOperationConcurrency != nil {
		workerOptions.MaxOperationConcurrency = *options.Config.Worker.MaxOperationConcurrency
	}
	if options.Config.Worker.MaxOperationRetryCount != nil {
		workerOptions.MaxOperationRetryCount = *options.Config.Worker.MaxOperationRetryCount
	}

	return &Service{
		options: options,
		Service: worker.Service{
			OperationStatusManager: options.StatusManager,
			Options:                workerOptions,
			QueueProvider:          options.QueueProvider,
			StorageProvider:        options.StorageProvider,
		},
		recipes: options.Recipes,
	}
}

// Name returns the name of the service used for logging.
func (w *Service) Name() string {
	return "dynamic-rp async worker"
}

// Run runs the service.
func (w *Service) Run(ctx context.Context) error {
	err := w.registerControllers(ctx)
	if err != nil {
		return err
	}

	return w.Start(ctx)
}

func (w *Service) registerControllers(ctx context.Context) error {
	// No controllers yet.
	return nil
}
