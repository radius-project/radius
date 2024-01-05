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
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

const (
	UCPProviderName = "System.Resources"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
}

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: UCPProviderName,
			Options:      options,
		},
	}
}

// Name returns a string containing the UCPProviderName and the text "async worker".
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", UCPProviderName)
}

// Run starts the service and worker. It initializes the service and sets the worker options based on the configuration,
// then starts the service with the given worker options. It returns an error if the initialization fails.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	workerOpts := worker.Options{}
	if w.Options.Config.WorkerServer != nil {
		if w.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *w.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if w.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *w.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
	}

	err := RegisterControllers(ctx, w.Controllers, opts)
	if err != nil {
		return err
	}

	return w.Start(ctx, workerOpts)
}

// RegisterControllers registers the controllers for the UCP backend.
func RegisterControllers(ctx context.Context, registry *worker.ControllerRegistry, opts ctrl.Options) error {
	err := registry.Register(ctx, v20231001preview.ResourceType, v1.OperationMethod(datamodel.OperationProcess), resourcegroups.NewTrackedResourceProcessController, opts)
	if err != nil {
		return err
	}

	return nil
}
