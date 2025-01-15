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

	appModel, err := model.NewApplicationModel(w.options.Arm, k8s.RuntimeClient, k8s.ClientSet, k8s.DiscoveryClient, k8s.DynamicClient)
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
				return deployment.NewDeploymentProcessor(appModel, w.DatabaseClient, k8s.RuntimeClient, k8s.ClientSet)
			},
		}

		err := b.ApplyAsyncHandler(ctx, w.Controllers(), opts)
		if err != nil {
			panic(err)
		}
	}

	return w.Start(ctx)
}
