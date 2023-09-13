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
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/builder"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/corerp/backend/deployment"
	"github.com/radius-project/radius/pkg/corerp/model"
	"github.com/radius-project/radius/pkg/kubeutil"
)

// AsyncWorker is a service to run AsyncReqeustProcessWorker.
type AsyncWorker struct {
	worker.Service

	handlerBuilder []builder.Builder
}

// NewAsyncWorker creates new service instance to run AsyncReqeustProcessWorker.
func NewAsyncWorker(options hostoptions.HostOptions, builder []builder.Builder) *AsyncWorker {
	return &AsyncWorker{
		Service: worker.Service{
			ProviderName: "radius",
			Options:      options,
		},
		handlerBuilder: builder,
	}
}

// Name represents the service name.
func (w *AsyncWorker) Name() string {
	return "radiusasyncworker"
}

// Run starts the service and worker.
func (w *AsyncWorker) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	k8s, err := kubeutil.NewClients(w.Options.K8sConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize kubernetes clients: %w", err)
	}

	appModel, err := model.NewApplicationModel(w.Options.Arm, k8s.RuntimeClient, k8s.ClientSet, k8s.DiscoveryClient)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	for _, b := range w.handlerBuilder {
		opts := ctrl.Options{
			DataProvider: w.StorageProvider,
			KubeClient:   k8s.RuntimeClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return deployment.NewDeploymentProcessor(appModel, w.StorageProvider, k8s.RuntimeClient, k8s.ClientSet)
			},
		}

		err := b.ApplyAsyncHandler(ctx, w.Controllers, opts)
		if err != nil {
			panic(err)
		}
	}

	workerOpts := worker.Options{}
	if w.Options.Config.WorkerServer != nil {
		if w.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *w.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if w.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *w.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	return w.Start(ctx, workerOpts)
}
