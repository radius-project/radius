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
	kubeclient "github.com/project-radius/radius/pkg/kubernetes/client"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// Service is the base worker service implementation to initialize and start web service.
type Service struct {
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
	// KubeClient is the Kubernetes controller runtime client.
	KubeClient controller_runtime.Client
	// SecretClient is the client to fetch secrets.
	SecretClient sv.SecretValueClient
}

// Init initializes web service.
func (s *Service) Init(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	s.StorageProvider = dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)

	qp := qprovider.New(s.ProviderName, s.Options.Config.QueueProvider)

	opSC, err := s.StorageProvider.GetStorageClient(ctx, s.ProviderName+"/operationstatuses")
	if err != nil {
		return err
	}
	reqQueueClient, err := qp.GetClient(ctx)
	if err != nil {
		return err
	}
	s.OperationStatusManager = manager.New(opSC, reqQueueClient, s.ProviderName, s.Options.Config.Env.RoleLocation)

	s.KubeClient, err = kubeclient.CreateKubeClient(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	if s.Options.Arm != nil {
		s.SecretClient = sv.NewSecretValueClient(*s.Options.Arm)
	}

	// Initialize the manager for ARM client cert validation
	if s.Options.Config.Server.EnableArmAuth {
		s.ARMCertManager = authentication.NewArmCertManager(s.Options.Config.Server.ArmMetadataEndpoint, logger)
		if err := s.ARMCertManager.Start(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Start starts HTTP server.
func (s *Service) Start(ctx context.Context, opt Options) error {
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
