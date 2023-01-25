// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"

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

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: providerName,
			Options:      options,
		},
	}
}

// Name represents the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", providerName)
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	coreAppModel, err := model.NewApplicationModel(w.Options.Arm, w.KubeClient, w.KubeClientSet)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	secretClient := sv.NewSecretValueClient(w.Options.Arm)
	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
		KubeClient:   w.KubeClient,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(coreAppModel, w.StorageProvider, secretClient, w.KubeClient, w.KubeClientSet)
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
