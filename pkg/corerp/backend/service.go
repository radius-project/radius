// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/backend/server"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
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

	controllers := server.NewControllerRegistry(sp)

	// TODO: register async operation controllers.
	// controllers.Register(ctx, "APPLICATIONSCORE.ENVIRONMENTS.PUT", "Applications.Core/environments", NewAsyncCreateOrUpdateEnvironment)

	worker := server.NewAsyncRequestProcessWorker(w.options, sp, controllers)

	logger.Info("Start AsyncRequestProcessWorker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Sorker stopped...")
	return nil
}
