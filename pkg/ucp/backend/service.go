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
	"net/http"
	"net/url"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/resourceproviders"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
	options *ucp.Options
}

// NewService creates new backend service instance to run the async worker.
func NewService(options *ucp.Options) *Service {
	return &Service{
		options: options,
		Service: worker.Service{
			// Will be initialized later.

		},
	}
}

// Name returns the service name.
func (w *Service) Name() string {
	return "ucp async worker"
}

// Run starts the background worker.
func (w *Service) Run(ctx context.Context) error {
	if w.options.Config.Worker.MaxOperationConcurrency != nil {
		w.Service.Options.MaxOperationConcurrency = *w.options.Config.Worker.MaxOperationConcurrency
	}
	if w.options.Config.Worker.MaxOperationRetryCount != nil {
		w.Service.Options.MaxOperationRetryCount = *w.options.Config.Worker.MaxOperationRetryCount
	}

	databaseClient, err := w.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	queueClient, err := w.options.QueueProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	w.Service.DatabaseClient = databaseClient
	w.Service.QueueClient = queueClient
	w.Service.OperationStatusManager = w.options.StatusManager

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	defaultDownstream, err := url.Parse(w.options.Config.Routing.DefaultDownstreamEndpoint)
	if err != nil {
		return err
	}

	transport := otelhttp.NewTransport(http.DefaultTransport)
	err = RegisterControllers(w.Controllers(), w.options.UCP, transport, opts, defaultDownstream)
	if err != nil {
		return err
	}

	return w.Start(ctx)
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
