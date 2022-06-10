// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"

	"github.com/go-logr/logr"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/queue"
	qprovider "github.com/project-radius/radius/pkg/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
)

type BaseService struct {
	ProviderName           string
	Options                hostoptions.HostOptions
	StorageProvider        dataprovider.DataStorageProvider
	OperationStatusManager manager.StatusManager
	Controllers            *ControllerRegistry
	QueueClient            queue.Client
}

func (s *BaseService) Init(ctx context.Context) error {
	s.StorageProvider = dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)

	if s.Options.Config.QueueProvider.Provider == qprovider.TypeInmemory {
		s.Options.Config.QueueProvider.InMemory = &qprovider.InMemoryQueueOptions{Name: s.ProviderName}
	}

	qp := qprovider.New(s.Options.Config.QueueProvider)
	opSC, err := s.StorageProvider.GetStorageClient(ctx, s.ProviderName+"/operationstatuses")
	if err != nil {
		return err
	}
	s.QueueClient, err = qp.GetClient(ctx)
	if err != nil {
		return err
	}
	s.OperationStatusManager = manager.New(opSC, s.QueueClient, s.ProviderName, s.Options.Config.Env.RoleLocation)
	s.Controllers = NewControllerRegistry(s.StorageProvider)
	return nil
}

func (s *BaseService) StartServer(ctx context.Context, opt Options) error {
	logger := logr.FromContextOrDiscard(ctx)
	ctx = hostoptions.WithContext(ctx, s.Options.Config)

	// Create and start worker.
	worker := New(opt, s.OperationStatusManager, s.QueueClient, s.Controllers)

	logger.Info("Start Worker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Sorker stopped...")
	return nil
}
