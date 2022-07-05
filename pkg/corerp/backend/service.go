// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	containers_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller/containers"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/model"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

const (
	providerName = "Applications.Core"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
}

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: providerName,
			Options:      options,
		},
	}
}

// Name represents the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", providerName)
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	if w.Options.Arm != nil {
		return errors.New("arm options cannot be empty")
	}

	scheme := clientgoscheme.Scheme
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))
	utilruntime.Must(contourv1.AddToScheme(scheme))

	// Create k8s
	k8s, err := controller_runtime.New(w.Options.K8sConfig, controller_runtime.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	// Create storageProvider and storageClient
	storageProvider := dataprovider.NewStorageProvider(w.Options.Config.StorageProvider)
	storageClient, err := storageProvider.GetStorageClient(ctx, string(w.Options.Config.StorageProvider.Provider))
	if err != nil {
		return err
	}

	// Create secretClient
	secretClient := renderers.NewSecretValueClient(*w.Options.Arm)

	// Create Core AppModel
	coreAppModel, err := model.NewApplicationModel(w.Options.Arm, k8s)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		StorageClient: storageClient,
		DataProvider:  storageProvider,
		SecretClient:  secretClient,
		KubeClient:    k8s,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(coreAppModel, storageProvider, secretClient, k8s)
		},
	}

	// Register controllers
	err = w.Controllers.Register(
		ctx,
		containers_ctrl.ResourceTypeName,
		v1.OperationPut,
		containers_ctrl.NewUpdateContainer,
		opts)
	if err != nil {
		panic(err)
	}

	return w.Start(ctx, worker.Options{})
}
