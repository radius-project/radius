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

package builder

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	asyncctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"
)

// Builder can be used to register operations and build HTTP routing paths and handlers for a resource namespace.
type Builder struct {
	namespaceNode *Namespace
	registrations []*OperationRegistration
}

// defaultHandlerOptions returns HandlerOption for the default operations such as getting operationStatuses and
// operationResults.
func defaultHandlerOptions(
	ctx context.Context,
	rootRouter chi.Router,
	rootScopePath string,
	namespace string,
	availableOperations []v1.Operation,
	ctrlOpts apictrl.Options) []server.HandlerOptions {
	namespace = strings.ToLower(namespace)

	handlers := []server.HandlerOptions{}
	if len(availableOperations) > 0 {
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		handlers = append(handlers, server.HandlerOptions{
			ParentRouter: rootRouter,
			Path:         rootScopePath + "/providers/" + namespace + "/operations",
			ResourceType: namespace + "/operations",
			Method:       v1.OperationGet,
			ControllerFactory: func(op apictrl.Options) (apictrl.Controller, error) {
				return defaultoperation.NewGetOperations(op, availableOperations)
			},
		})
	}

	statusType := namespace + "/operationstatuses"
	resultType := namespace + "/operationresults"
	handlers = append(handlers, server.HandlerOptions{
		ParentRouter:      rootRouter,
		Path:              fmt.Sprintf("%s/providers/%s/locations/{location}/operationstatuses/{operationId}", rootScopePath, namespace),
		ResourceType:      statusType,
		Method:            v1.OperationGet,
		ControllerFactory: defaultoperation.NewGetOperationStatus,
	})

	handlers = append(handlers, server.HandlerOptions{
		ParentRouter:      rootRouter,
		Path:              fmt.Sprintf("%s/providers/%s/locations/{location}/operationresults/{operationId}", rootScopePath, namespace),
		ResourceType:      resultType,
		Method:            v1.OperationGet,
		ControllerFactory: defaultoperation.NewGetOperationResult,
	})

	return handlers
}

func (b *Builder) Namespace() string {
	return b.namespaceNode.Name
}

const (
	UCPRootScopePath  = "/planes/radius/{planeName}"
	ResourceGroupPath = "/resourcegroups/{resourceGroupName}"
)

// NewOpenAPIValidatorMiddleware creates a new OpenAPI validator middleware.
func NewOpenAPIValidator(ctx context.Context, base, namespace string) (func(h http.Handler) http.Handler, error) {
	rootScopePath := base + UCPRootScopePath

	// URLs may use either the subscription/plane scope or resource group scope.
	// These paths are order sensitive and the longer path MUST be registered first.
	prefixes := []string{
		rootScopePath + ResourceGroupPath,
		rootScopePath,
	}

	specLoader, err := validator.LoadSpec(ctx, namespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return nil, err
	}

	return validator.APIValidator(validator.Options{
		SpecLoader:         specLoader,
		ResourceTypeGetter: validator.RadiusResourceTypeGetter,
	}), nil
}

// ApplyAPIHandlers builds HTTP routing paths and handlers for namespace.
func (b *Builder) ApplyAPIHandlers(ctx context.Context, r chi.Router, ctrlOpts apictrl.Options, middlewares ...func(h http.Handler) http.Handler) error {
	rootScopePath := ctrlOpts.PathBase + UCPRootScopePath

	// Configure the default handlers.
	handlerOptions := defaultHandlerOptions(ctx, r, rootScopePath, b.namespaceNode.Name, b.namespaceNode.availableOperations, ctrlOpts)

	routerMap := map[string]chi.Router{}
	for _, h := range b.registrations {
		if h == nil {
			continue
		}

		key := ""
		route := ""
		switch h.Method {
		case v1.OperationPlaneScopeList:
			route = fmt.Sprintf("%s/providers/%s", rootScopePath, strings.ToLower(h.ResourceType))
			key = "plane-" + h.ResourceType
		case v1.OperationList:
			route = fmt.Sprintf("%s/resourcegroups/{resourceGroupName}/providers/%s", rootScopePath, h.ResourceNamePattern)
			key = "rg-" + h.ResourceType
		default:
			route = fmt.Sprintf("%s/resourcegroups/{resourceGroupName}/providers/%s", rootScopePath, h.ResourceNamePattern)
			key = "resource-" + h.ResourceNamePattern
		}

		if _, ok := routerMap[key]; !ok {
			routerMap[key] = server.NewSubrouter(r, route, middlewares...)
		}

		handlerOptions = append(handlerOptions, server.HandlerOptions{
			ParentRouter:      routerMap[key],
			Path:              strings.ToLower(h.Path),
			ResourceType:      h.ResourceType,
			Method:            h.Method,
			ControllerFactory: h.APIController,
		})
	}

	for _, o := range handlerOptions {
		if err := server.RegisterHandler(ctx, o, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// ApplyAsyncHandler registers asynchronous controllers from HandlerOutput.
func (b *Builder) ApplyAsyncHandler(ctx context.Context, registry *worker.ControllerRegistry, ctrlOpts asyncctrl.Options) error {
	for _, h := range b.registrations {
		if h == nil {
			continue
		}

		if h.AsyncController != nil {
			err := registry.Register(ctx, h.ResourceType, h.Method, h.AsyncController, ctrlOpts)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
