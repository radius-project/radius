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
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
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
	SecretClient renderers.SecretValueClient
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
	s.OperationStatusManager = manager.New(opSC, reqQueueClient, s.ProviderName, s.Options.Config.Environment.RoleLocation)

	// TODO: Instead of creating client in startup, it is better for controller to create clients.
	if s.Options.K8sConfig != nil {
		scheme := clientgoscheme.Scheme
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(csidriver.AddToScheme(scheme))
		utilruntime.Must(contourv1.AddToScheme(scheme))

		k8s, err := controller_runtime.New(s.Options.K8sConfig, controller_runtime.Options{Scheme: scheme})
		if err != nil {
			return err
		}
		s.KubeClient = k8s
	}

	if s.Options.Arm != nil {
		s.SecretClient = renderers.NewSecretValueClient(*s.Options.Arm)
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
