// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/backend/controller/containers"
	"github.com/project-radius/radius/pkg/corerp/backend/server"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue/inmemory"

	containers_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	options hostoptions.HostOptions
}

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		options: options,
	}
}

// Name represents the service name.
func (w *Service) Name() string {
	return "async request process worker"
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	sp := dataprovider.NewStorageProvider(w.options.Config.StorageProvider)
	ctx = hostoptions.WithContext(ctx, w.options.Config)

	// Register async operation controllers.
	controllers := server.NewControllerRegistry(sp)
	err := controllers.Register(
		ctx,
		asyncoperation.OperationType{TypeName: containers_ctrl.ResourceTypeName, Method: asyncoperation.OperationGet},
		containers.NewUpdateContainer)
	if err != nil {
		panic(err)
	}

	// Create Async operation manager.
	sc, err := sp.GetStorageClient(ctx, provider_ctrl.OperationStatusResourceTypeName)
	if err != nil {
		panic(err)
	}
	asyncOpManager := asyncoperation.NewStatusManager(sc, nil, "applications.core", w.options.Config.Env.RoleLocation)

	// TODO: Make it configurable.
	queue := inmemory.NewClient(nil)

	// Create and start worker.
	worker := server.NewAsyncRequestProcessWorker(w.options, asyncOpManager, queue, controllers)

	logger.Info("Start AsyncRequestProcessWorker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Sorker stopped...")
	return nil
}
