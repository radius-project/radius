// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"

	containers_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller/containers"
)

const (
	providerName = "Applications.Core"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.BaseWorkerService
}

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.BaseWorkerService{
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

	// Register controllers
	err := w.Controllers.Register(
		ctx,
		v1.OperationType{Type: containers_ctrl.ResourceTypeName, Method: v1.OperationPut},
		containers_ctrl.NewUpdateContainer)
	if err != nil {
		panic(err)
	}

	return w.StartServer(ctx, worker.Options{})
}
