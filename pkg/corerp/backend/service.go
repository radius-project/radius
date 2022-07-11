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

	containers_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller/containers"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/model"
)

const (
	providerName = "Applications.Core"
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

	coreAppModel, err := model.NewApplicationModel(w.Options.Arm, w.KubeClient)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
		SecretClient: w.SecretClient,
		KubeClient:   w.KubeClient,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(coreAppModel, w.StorageProvider, w.SecretClient, w.KubeClient)
		},
	}

	// Register controllers
	err = w.Controllers.Register(
		ctx,
		containers_ctrl.ResourceTypeName,
		v1.OperationPut,
		containers_ctrl.NewUpdateContainer,
		opts)
	if err != nil {
		panic(err)
	}

	return w.Start(ctx, worker.Options{})
}
