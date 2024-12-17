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

package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/resourceproviders"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	ucpoptions "github.com/radius-project/radius/pkg/ucp/hostoptions"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	UCPProviderName = "System.Resources"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service

	config ucpoptions.UCPConfig
}

// NewService creates new service instance to run AsyncRequestProcessWorker.
func NewService(options hostoptions.HostOptions, config ucpoptions.UCPConfig) *Service {
	return &Service{
		Service: worker.Service{
			ProviderName: UCPProviderName,
			Options:      options,
		},
		config: config,
	}
}

// Name returns a string containing the UCPProviderName and the text "async worker".
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", UCPProviderName)
}

// Run starts the service and worker. It initializes the service and sets the worker options based on the configuration,
// then starts the service with the given worker options. It returns an error if the initialization fails.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
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

	databaseClient, err := w.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	defaultDownstream, err := url.Parse(w.config.Routing.DefaultDownstreamEndpoint)
	if err != nil {
		return err
	}

	transport := otelhttp.NewTransport(http.DefaultTransport)
	err = RegisterControllers(w.Controllers, w.Options.UCPConnection, transport, opts, defaultDownstream)
	if err != nil {
		return err
	}

	return w.Start(ctx, workerOpts)
}

// RegisterControllers registers the controllers for the UCP backend.
func RegisterControllers(registry *worker.ControllerRegistry, connection sdk.Connection, transport http.RoundTripper, opts ctrl.Options, defaultDownstream *url.URL) error {
	// Tracked resources
	err := errors.Join(nil, registry.Register(v20231001preview.ResourceType, v1.OperationMethod(datamodel.OperationProcess), func(opts ctrl.Options) (ctrl.Controller, error) {
		return resourcegroups.NewTrackedResourceProcessController(opts, transport, defaultDownstream)
	}, opts))

	// Resource providers and related types
	err = errors.Join(err, registry.Register(datamodel.ResourceProviderResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.ResourceProviderResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.ResourceTypeResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypePutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.ResourceTypeResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypeDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.APIVersionResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.APIVersionResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.LocationResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(datamodel.LocationResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))

	if err != nil {
		return err
	}

	return nil
}
