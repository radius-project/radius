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

package server

import (
	"context"
	"fmt"
	"os"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/builder"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/corerp/backend/deployment"
	"github.com/radius-project/radius/pkg/corerp/model"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// AsyncWorker is a service to run AsyncRequestProcessWorker.
type AsyncWorker struct {
	worker.Service

	options        hostoptions.HostOptions
	handlerBuilder []builder.Builder
}

// NewAsyncWorker creates new service instance to run AsyncRequestProcessWorker.
func NewAsyncWorker(options hostoptions.HostOptions, builder []builder.Builder) *AsyncWorker {
	return &AsyncWorker{
		options:        options,
		handlerBuilder: builder,
		Service:        worker.Service{
			// Will be initialized later
		},
	}
}

// Name represents the service name.
func (w *AsyncWorker) Name() string {
	return "radiusasyncworker"
}

func (w *AsyncWorker) init(ctx context.Context) error {
	workerOptions := worker.Options{}
	if w.options.Config.WorkerServer != nil {
		if w.options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOptions.MaxOperationConcurrency = *w.options.Config.WorkerServer.MaxOperationConcurrency
		}
		if w.options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOptions.MaxOperationRetryCount = *w.options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	queueProvider := queueprovider.New(w.options.Config.QueueProvider)
	databaseProvider := databaseprovider.FromOptions(w.options.Config.DatabaseProvider)

	databaseClient, err := databaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	queueClient, err := queueProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	statusManager := statusmanager.New(databaseClient, queueClient, w.options.Config.Env.RoleLocation)

	w.Service = worker.Service{
		DatabaseClient:         databaseClient,
		OperationStatusManager: statusManager,
		Options:                workerOptions,
		QueueClient:            queueClient,
	}

	return nil
}

// Run starts the service and worker.
func (w *AsyncWorker) Run(ctx context.Context) error {
	k8s, err := kubeutil.NewClients(w.options.K8sConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize kubernetes clients: %w", err)
	}

	// deployTarget holds the Kubernetes clients used to apply application output
	// resources (Deployments, Services, Secrets, etc.). By default this is the
	// control-plane cluster Radius runs on. When RADIUS_TARGET_KUBECONFIG is set,
	// output resources are applied to the external cluster named by that
	// kubeconfig, so the whole application lands on the target cluster while
	// Radius's own bookkeeping (KubeClient below) stays on the control plane.
	deployTarget, err := w.deploymentTargetClients(ctx, k8s)
	if err != nil {
		return err
	}

	appModel, err := model.NewApplicationModel(w.options.Arm, deployTarget.RuntimeClient, deployTarget.ClientSet, deployTarget.DiscoveryClient, deployTarget.DynamicClient)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	err = w.init(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize async worker: %w", err)
	}

	for _, b := range w.handlerBuilder {
		opts := ctrl.Options{
			DatabaseClient: w.DatabaseClient,
			KubeClient:     k8s.RuntimeClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return deployment.NewDeploymentProcessor(appModel, w.DatabaseClient, deployTarget.RuntimeClient, deployTarget.ClientSet)
			},
		}

		err := b.ApplyAsyncHandler(ctx, w.Controllers(), opts)
		if err != nil {
			panic(err)
		}
	}

	return w.Start(ctx)
}

// deploymentTargetClients returns the Kubernetes clients that application output
// resources should be deployed with. It returns the control-plane clients
// unchanged unless RADIUS_TARGET_KUBECONFIG is set, in which case it builds
// clients for the external cluster named by that kubeconfig. The injected
// kubeconfig is owned and refreshed out of band (by the workflow that mounts it);
// Radius reads it fresh on startup and never persists it.
func (w *AsyncWorker) deploymentTargetClients(ctx context.Context, controlPlane *kubeutil.Clients) (*kubeutil.Clients, error) {
	targetKubeconfigPath := os.Getenv(kubeutil.TargetKubeconfigEnvVar)
	if targetKubeconfigPath == "" {
		return controlPlane, nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Deploying application output resources to external target cluster",
		"kubeconfig", targetKubeconfigPath, "envVar", kubeutil.TargetKubeconfigEnvVar)

	targetConfig, err := kubeutil.NewClientConfigForTargetCluster(targetKubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load target kubeconfig from %s=%q: %w", kubeutil.TargetKubeconfigEnvVar, targetKubeconfigPath, err)
	}

	targetClients, err := kubeutil.NewClients(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize target cluster kubernetes clients: %w", err)
	}

	return targetClients, nil
}
