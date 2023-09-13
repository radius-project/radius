/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package worker

import (
	"context"

	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	qprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
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
	// KubeClientSet is the Kubernetes client.
	KubeClientSet kubernetes.Interface
	// KubeDiscoveryClient is the Kubernetes discovery client.
	KubeDiscoveryClient discovery.ServerResourcesInterface
}

// Init initializes worker service - it initializes the StorageProvider, RequestQueue, OperationStatusManager, Controllers, KubeClient and
// returns an error if any of these operations fail.
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

	if s.Options.K8sConfig != nil {
		s.KubeClient, err = kubeutil.NewRuntimeClient(s.Options.K8sConfig)
		if err != nil {
			return err
		}

		s.KubeClientSet, err = kubernetes.NewForConfig(s.Options.K8sConfig)
		if err != nil {
			return err
		}

		discoveryClient, err := discovery.NewDiscoveryClientForConfig(s.Options.K8sConfig)
		if err != nil {
			return err
		}

		// Use legacy discovery client to avoid the issue of the staled GroupVersion discovery(api.ucp.dev/v1alpha3).
		// TODO: Disable UseLegacyDiscovery once https://github.com/radius-project/radius/issues/5974 is resolved.
		discoveryClient.UseLegacyDiscovery = true
		s.KubeDiscoveryClient = discoveryClient
	}
	return nil
}

// Start creates and starts a worker, and logs any errors that occur while starting the worker.
func (s *Service) Start(ctx context.Context, opt Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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
