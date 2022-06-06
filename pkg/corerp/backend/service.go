// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	sm "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue/inmemory"

	containers_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller/containers"
)

const (
	providerName         = "Applications.Core"
	providerResourceType = providerName + "/provider"
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
	return fmt.Sprintf("%s async worker", providerName)
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	sp := dataprovider.NewStorageProvider(w.options.Config.StorageProvider)
	ctx = hostoptions.WithContext(ctx, w.options.Config)

	// Register async operation controllers.
	controllers := worker.NewControllerRegistry(sp)
	err := controllers.Register(
		ctx,
		v1.OperationType{Type: containers_ctrl.ResourceTypeName, Method: v1.OperationGet},
		containers_ctrl.NewUpdateContainer)
	if err != nil {
		panic(err)
	}

	// Create Async operation manager.
	sc, err := sp.GetStorageClient(ctx, providerResourceType)
	if err != nil {
		panic(err)
	}
	asyncOpManager := sm.New(sc, nil, providerName, w.options.Config.Env.RoleLocation)

	// TODO: Make it configurable.
	queue := inmemory.NewClient(nil)

	// Create and start worker.
	worker := worker.New(worker.Options{}, asyncOpManager, queue, controllers)

	logger.Info("Start Worker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Sorker stopped...")
	return nil
}
