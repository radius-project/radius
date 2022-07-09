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
	kubeclient "github.com/project-radius/radius/pkg/kubernetes/client"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"

	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// Service is the base worker service implementation to initialize and start worker.
type Service struct {
	// ProviderName is the name of provider namespace.
	ProviderName string
	// Options is the server hosting options.
	Options hostoptions.HostOptions
	// StorageProvider is the provider of storage client.
	StorageProvider dataprovider.DataStorageProvider
	// OperationStatusManager is the manager of the operation status.
	OperationStatusManager manager.StatusManager
	// Controllers is the registry of the async operation controllers.
	Controllers *ControllerRegistry
	// RequestQueue is the queue client for async operation request message.
	RequestQueue queue.Client
	// KubeClient is the Kubernetes controller runtime client.
	KubeClient controller_runtime.Client
	// SecretClient is the client to fetch secrets.
	SecretClient renderers.SecretValueClient
}

// Init initializes worker service.
func (s *Service) Init(ctx context.Context) error {
	s.StorageProvider = dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)
	qp := qprovider.New(s.ProviderName, s.Options.Config.QueueProvider)
	opSC, err := s.StorageProvider.GetStorageClient(ctx, s.ProviderName+"/operationstatuses")
	if err != nil {
		return err
	}
	s.RequestQueue, err = qp.GetClient(ctx)
	if err != nil {
		return err
	}
	s.OperationStatusManager = manager.New(opSC, s.RequestQueue, s.ProviderName, s.Options.Config.Env.RoleLocation)
	s.Controllers = NewControllerRegistry(s.StorageProvider)

	s.KubeClient, err = kubeclient.CreateKubeClient(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	if s.Options.Arm != nil {
		s.SecretClient = renderers.NewSecretValueClient(*s.Options.Arm)
	}

	return nil
}

// Start starts the worker.
func (s *Service) Start(ctx context.Context, opt Options) error {
	logger := logr.FromContextOrDiscard(ctx)
	ctx = hostoptions.WithContext(ctx, s.Options.Config)

	// Create and start worker.
	worker := New(opt, s.OperationStatusManager, s.RequestQueue, s.Controllers)

	logger.Info("Start Worker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
	}

	logger.Info("Worker stopped...")
	return nil
}
