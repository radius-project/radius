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

type Service struct {
	Options hostoptions.HostOptions
}

func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

func (w *Service) Name() string {
	return "async request process worker"
}

func (w *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	sp := dataprovider.NewStorageProvider(w.Options.Config.StorageProvider)
	ctx = hostoptions.WithContext(ctx, w.Options.Config)

	controllers := server.NewControllerRegistry(sp)

	// TODO: register handlers

	worker := server.NewAsyncRequestProcessWorker(w.Options, sp, controllers)

	logger.Info("Start AsyncRequestProcessWorker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Server stopped...")
	return nil
}
