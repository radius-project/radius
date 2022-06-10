// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/authentication"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	qprovider "github.com/project-radius/radius/pkg/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
)

// BaseWorkerService is the base worker service implementation to initialize the start worker.
type BaseService struct {
	// ProviderName is the name of provider namespace.
	ProviderName string
	// Options is the server hosting options.
	Options hostoptions.HostOptions
	// StorageProvider is the provider of storage client.
	StorageProvider dataprovider.DataStorageProvider
	// OperationStatusManager is the manager of the operation status.
	OperationStatusManager manager.StatusManager
	// ARMCertManager is the certificate manager of client cert authentication.
	ARMCertManager *authentication.ArmCertManager
}

// Init initializes web service.
func (s *BaseService) Init(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	s.StorageProvider = dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)

	if s.Options.Config.QueueProvider.Provider == qprovider.TypeInmemory {
		s.Options.Config.QueueProvider.InMemory = &qprovider.InMemoryQueueOptions{Name: s.ProviderName}
	}

	qp := qprovider.New(s.Options.Config.QueueProvider)

	opSC, err := s.StorageProvider.GetStorageClient(ctx, s.ProviderName+"/operationstatuses")
	if err != nil {
		return err
	}
	qcli, err := qp.GetClient(ctx)
	if err != nil {
		return err
	}
	s.OperationStatusManager = manager.New(opSC, qcli, s.ProviderName, s.Options.Config.Env.RoleLocation)

	// Initialize the manager for ARM client cert validation
	if s.Options.Config.Server.EnableArmAuth {
		s.ARMCertManager = authentication.NewArmCertManager(s.Options.Config.Server.ArmMetadataEndpoint, logger)
		if err := s.ARMCertManager.Start(ctx); err != nil {
			return err
		}
	}

	return nil
}

// StartServer starts HTTP server.
func (s *BaseService) StartServer(ctx context.Context, opt Options) error {
	logger := logr.FromContextOrDiscard(ctx)
	ctx = hostoptions.WithContext(ctx, s.Options.Config)

	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)
	server, err := New(ctx, opt)
	if err != nil {
		return err
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", address))
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		// We expect this, safe to ignore.
		logger.Info("Server stopped...")
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Server stopped...")
	return nil
}
