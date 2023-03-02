// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
)

const (
	UCPProviderName = "ucp"
)

var (
	// ResourceTypeNames is the array that holds resource types that needs async processing.
	// We use this array to generate generic backend controller for each resource.
	ResourceTypeNames = []string{
		//TODO: Add resource type names here.
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
			ProviderName: UCPProviderName,
			Options:      options,
		},
	}
}

// Name represents the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", UCPProviderName)
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	// Register controllers
	for range ResourceTypeNames {
		// TODO Add backend controller for each resource type.
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
