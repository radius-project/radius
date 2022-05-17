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
	"github.com/project-radius/radius/pkg/jobqueue"
	jq "github.com/project-radius/radius/pkg/jobqueue/inmemory"
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
	jqClient := jq.NewClient()

	worker := server.NewAsyncRequestProcessor(w.Options, sp, jqClient)
	// TODO: Register controllers.
	worker.Start(ctx)

	logger.Info("Server stopped...")
	return nil
}

func (w *Service) getOperationController(ctx context.Context, job *jobqueue.JobMessageResponse) error {
	return nil
}

func (w *Service) runOperation(ctx context.Context, job *jobqueue.JobMessageResponse) error {
	return nil
}
