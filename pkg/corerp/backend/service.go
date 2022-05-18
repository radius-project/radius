// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"

	"github.com/go-logr/logr"
	asynctrl_environments "github.com/project-radius/radius/pkg/corerp/backend/controller/environments"
	"github.com/project-radius/radius/pkg/corerp/backend/server"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	jq "github.com/project-radius/radius/pkg/queue/inmemory"
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

	handlers := server.NewHandlerRegistry(sp)
	handlers.RegisterController(
		ctx, "APPLICATIONSCORE.ENVIRONMENT.PUT", "applications.core/environments",
		asynctrl_environments.NewCreateOrUpdateEnvironmentAsync)

	inmemRequestQueueClient := jq.NewClient()

	worker := server.NewAsyncRequestProcessor(w.Options, sp, inmemRequestQueueClient, handlers)
	worker.Start(ctx)

	logger.Info("Server stopped...")
	return nil
}
