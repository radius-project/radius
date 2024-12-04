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
	workerOptions := worker.Options{}
	if options.Config.Worker.MaxOperationConcurrency != nil {
		workerOptions.MaxOperationConcurrency = *options.Config.Worker.MaxOperationConcurrency
	}
	if options.Config.Worker.MaxOperationRetryCount != nil {
		workerOptions.MaxOperationRetryCount = *options.Config.Worker.MaxOperationRetryCount
	}
	return &Service{
		options: options,
		Service: worker.Service{
			OperationStatusManager: options.StatusManager,
			Options:                workerOptions,
			QueueProvider:          options.QueueProvider,
			StorageProvider:        options.StorageProvider,
		},
	}
}

// Name returns the service name.
func (w *Service) Name() string {
	return "ucp async worker"
}

// Run starts the background worker.
func (w *Service) Run(ctx context.Context) error {
	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
	}

	defaultDownstream, err := url.Parse(w.options.Config.Routing.DefaultDownstreamEndpoint)
	if err != nil {
		return err
	}

	transport := otelhttp.NewTransport(http.DefaultTransport)
	err = RegisterControllers(ctx, w.Controllers(), w.options.UCP, transport, opts, defaultDownstream)
	if err != nil {
		return err
	}

	return w.Start(ctx)
}

// RegisterControllers registers the controllers for the UCP backend.
func RegisterControllers(ctx context.Context, registry *worker.ControllerRegistry, connection sdk.Connection, transport http.RoundTripper, opts ctrl.Options, defaultDownstream *url.URL) error {
	// Tracked resources
	err := errors.Join(nil, registry.Register(ctx, v20231001preview.ResourceType, v1.OperationMethod(datamodel.OperationProcess), func(opts ctrl.Options) (ctrl.Controller, error) {
		return resourcegroups.NewTrackedResourceProcessController(opts, transport, defaultDownstream)
	}, opts))

	// Resource providers and related types
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceProviderResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceProviderResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceTypeResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypePutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceTypeResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypeDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.APIVersionResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.APIVersionResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.LocationResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.LocationResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))

	if err != nil {
		return err
	}

	return nil
}
