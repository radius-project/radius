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

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
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
	return &Service{
		options: options,
		Service: worker.Service{
			// Will be initialized later
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
	if w.options.Config.Worker.MaxOperationConcurrency != nil {
		w.Service.Options.MaxOperationConcurrency = *w.options.Config.Worker.MaxOperationConcurrency
	}
	if w.options.Config.Worker.MaxOperationRetryCount != nil {
		w.Service.Options.MaxOperationRetryCount = *w.options.Config.Worker.MaxOperationRetryCount
	}

	databaseClient, err := w.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	queueClient, err := w.options.QueueProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	w.Service.DatabaseClient = databaseClient
	w.Service.QueueClient = queueClient
	w.Service.OperationStatusManager = w.options.StatusManager

	err = w.registerControllers()
	if err != nil {
		return err
	}

	return w.Start(ctx)
}

func (w *Service) registerControllers() error {
	options := ctrl.Options{
		DatabaseClient: w.Service.DatabaseClient,
	}

	return w.Service.Controllers().RegisterDefault(NewDynamicResourceController, options)
}
