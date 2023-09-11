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
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// APIVersion is a query string for the API version of Radius resource provider.
	APIVersionParam = "api-version"

	// CatchAllPath is the path for the catch-all route.
	CatchAllPath = "/*"
)

var (
	ErrInvalidOperationTypeOption = errors.New("the resource type and method must be specified if the operation type is not specified")
)

type ControllerFactoryFunc func(ctrl.Options) (ctrl.Controller, error)

// HandlerOptions represents a controller to be registered with the server.
//
// Each HandlerOptions should represent either a resource-type-scoped operation
// (e.g. GET on an `Applications.Core/controllers` resource) or a more general operation that works with
// multiple types of resources (e.g. PUT on any type of AWS resource):
// - Set ResourceType for operations that are scoped to a resource type.
// - Set OperationType for general operations.
//
// In the controller options passed to the controller factory:
//
// - When ResourceType is set, the StorageClient will be configured to use the resource type.
// - When OperationType is set, the StorageClient will be generic and not filtered to a specific resource type.
type HandlerOptions struct {
	// ParentRouter is the router to register the handler with.
	ParentRouter chi.Router

	// Path is the matched pattern for ParentRouter handler. This is optional and the default value is "/".
	Path string

	// ResourceType is the resource type of the operation. May be blank if Operation is specified.
	//
	// If specified the ResourceType will be used to filter the StorageClient.
	ResourceType string

	// Method is the method of the operation. May be blank if Operation is specified.
	//
	// If the specified the Method will be used to filter by HTTP method.
	Method v1.OperationMethod

	// OperationType designates the operation and should be unique per handler. May be blank if ResourceType and Method are specified.
	//
	// The OperationType is used in logs and other mechanisms to identify the kind of operation being performed.
	// If the OperationType is not specified, it will be inferred from that ResourceType and Method.
	OperationType *v1.OperationType

	// ControllerFactory is a function invoked to create the controller. Will be invoked once during server startup.
	ControllerFactory ControllerFactoryFunc

	// Middlewares are the middlewares to apply to the handler.
	Middlewares []func(http.Handler) http.Handler
}

// NewSubrouter creates a new subrouter and mounts it on the parent router with the given middlewares.
func NewSubrouter(parent chi.Router, path string, middlewares ...func(http.Handler) http.Handler) chi.Router {
	subrouter := chi.NewRouter()
	parent.Mount(path, subrouter)
	subrouter.Use(middlewares...)
	return subrouter
}

// HandlerForController creates a http.HandlerFunc function that runs resource provider frontend controller, renders a
// http response from the returned rest.Response, and handles the error as a default internal error if this controller returns error.
func HandlerForController(controller ctrl.Controller, operationType v1.OperationType) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		rpcCtx := v1.ARMRequestContextFromContext(ctx)
		// Set the operation type in the context.
		rpcCtx.OperationType = operationType

		// Add OTEL labels for the telemetry.
		withOtelLabelsForRequest(req)

		response, err := controller.Run(ctx, w, req)
		if err != nil {
			HandleError(ctx, w, req, err)
			return
		}

		// The response may be nil in some advanced cases like proxying to another server.
		if response != nil {
			err = response.Apply(ctx, w, req)
			if err != nil {
				HandleError(ctx, w, req, err)
				return
			}
		}
	}
}

// RegisterHandler registers a handler for the given resource type and method. This function should only
// be used for controllers that process a single resource type.
func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts ctrl.Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	if opts.OperationType == nil && (opts.ResourceType == "" || opts.Method == "") {
		return ErrInvalidOperationTypeOption
	}

	storageClient, err := ctrlOpts.DataProvider.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}

	ctrlOpts.StorageClient = storageClient
	ctrlOpts.ResourceType = opts.ResourceType

	ctrl, err := opts.ControllerFactory(ctrlOpts)
	if err != nil {
		return err
	}

	if opts.OperationType == nil {
		opts.OperationType = &v1.OperationType{Type: opts.ResourceType, Method: opts.Method}
	}

	if opts.Path == "" {
		opts.Path = "/"
	}

	// Ensure that the current route is not registered before. We logs the warning message if the route is registered before.
	duplicated := opts.ParentRouter.Match(chi.NewRouteContext(), opts.Method.HTTPMethod(), opts.Path)
	if duplicated {
		logger.Info(fmt.Sprintf("Warning: skipping handler registration because '%s %s' has been registered before.", opts.Method, opts.Path))
		return nil
	}

	handler := HandlerForController(ctrl, *opts.OperationType)
	namedRouter := opts.ParentRouter.With(opts.Middlewares...)
	if opts.Path == CatchAllPath {
		namedRouter.HandleFunc(opts.Path, handler)
	} else {
		namedRouter.MethodFunc(opts.OperationType.Method.HTTPMethod(), opts.Path, handler)
	}

	return nil
}

func withOtelLabelsForRequest(req *http.Request) {
	labeler, ok := otelhttp.LabelerFromContext(req.Context())
	if !ok {
		return
	}

	armContext := v1.ARMRequestContextFromContext(req.Context())
	resourceID := armContext.ResourceID

	if resourceID.IsResource() || resourceID.IsResourceCollection() {
		labeler.Add(attribute.String("resource_type", strings.ToLower(resourceID.Type())))
	}
}

// ConfigureDefaultHandlers registers handlers for the default operations such as getting operationStatuses and
// operationResults, and updating a subscription lifecycle. It returns an error if any of the handler registrations fail.
func ConfigureDefaultHandlers(
	ctx context.Context,
	rootRouter chi.Router,
	rootScopePath string,
	isAzureProvider bool,
	providerNamespace string,
	operationCtrlFactory ControllerFactoryFunc,
	ctrlOpts ctrl.Options) error {
	providerNamespace = strings.ToLower(providerNamespace)
	rt := providerNamespace + "/providers"

	if isAzureProvider {
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		err := RegisterHandler(ctx, HandlerOptions{
			ParentRouter:      rootRouter,
			Path:              "/providers/" + providerNamespace + "/operations",
			ResourceType:      rt,
			Method:            v1.OperationGet,
			ControllerFactory: operationCtrlFactory,
		}, ctrlOpts)
		if err != nil {
			return err
		}

		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
		err = RegisterHandler(ctx, HandlerOptions{
			ParentRouter:      rootRouter,
			Path:              rootScopePath,
			ResourceType:      rt,
			Method:            v1.OperationPut,
			ControllerFactory: defaultoperation.NewCreateOrUpdateSubscription,
		}, ctrlOpts)
		if err != nil {
			return err
		}
	}

	statusRT := providerNamespace + "/operationstatuses"
	opStatus := fmt.Sprintf("%s/providers/%s/locations/{location}/operationstatuses/{operationId}", rootScopePath, providerNamespace)
	err := RegisterHandler(ctx, HandlerOptions{
		ParentRouter:      rootRouter,
		Path:              opStatus,
		ResourceType:      statusRT,
		Method:            v1.OperationGet,
		ControllerFactory: defaultoperation.NewGetOperationStatus,
	}, ctrlOpts)
	if err != nil {
		return err
	}

	opResult := fmt.Sprintf("%s/providers/%s/locations/{location}/operationresults/{operationId}", rootScopePath, providerNamespace)
	err = RegisterHandler(ctx, HandlerOptions{
		ParentRouter:      rootRouter,
		Path:              opResult,
		ResourceType:      statusRT,
		Method:            v1.OperationGet,
		ControllerFactory: defaultoperation.NewGetOperationResult,
	}, ctrlOpts)
	if err != nil {
		return err
	}

	return nil
}

// HandleError handles unhandled errors from frontend controller and creates internal server error response based on the error type.
func HandleError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Error(err, "unhandled error")

	var response rest.Response
	// Try to use the ARM format to send back the error info
	// if the error is due to api conversion failure return bad resquest
	switch v := err.(type) {
	case *v1.ErrModelConversion:
		response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
				Message: err.Error(),
			},
		})
	case *v1.ErrClientRP:
		response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v.Code,
				Message: v.Message,
			},
		})
	default:
		if errors.Is(err, v1.ErrInvalidModelConversion) {
			response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
					Message: err.Error(),
				},
			})
		} else {
			response = rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: err.Error(),
				},
			})
		}
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		body := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: err.Error(),
			},
		}
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}
