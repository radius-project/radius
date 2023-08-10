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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"

	backend_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/model"
)

const (
	providerName = "Applications.Core"
)

var (
	// ResourceTypeNames is the array that holds resource types that needs async processing.
	// We use this array to generate generic backend controller for each resource.
	ResourceTypeNames = []string{
		"Applications.Core/containers",
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
		"Applications.Core/volumes",
	}
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
}

// # Function Explanation
//
// NewService creates a new Service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: providerName,
			Options:      options,
		},
	}
}

// # Function Explanation
//
// Name returns a string containing the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", providerName)
}

// # Function Explanation
//
// Run initializes the application model, registers controllers for different resource types, and starts the worker with
// the given options.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	coreAppModel, err := model.NewApplicationModel(w.Options.Arm, w.KubeClient, w.KubeClientSet)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
		KubeClient:   w.KubeClient,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(coreAppModel, w.StorageProvider, w.KubeClient, w.KubeClientSet)
		},
	}

	for _, rt := range ResourceTypeNames {
		// Register controllers
		err = w.Controllers.Register(ctx, rt, v1.OperationPut, backend_ctrl.NewCreateOrUpdateResource, opts)
		if err != nil {
			panic(err)
		}
		err = w.Controllers.Register(ctx, rt, v1.OperationPatch, backend_ctrl.NewCreateOrUpdateResource, opts)
		if err != nil {
			panic(err)
		}
		err = w.Controllers.Register(ctx, rt, v1.OperationDelete, backend_ctrl.NewDeleteResource, opts)
		if err != nil {
			panic(err)
		}
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

	return w.Start(ctx, workerOpts)
}
