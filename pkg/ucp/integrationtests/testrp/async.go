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

package testrp

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	backend_ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	frontend_ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/armrpc/servicecontext"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

type BackendFunc func(ctx context.Context, request *backend_ctrl.Request) (backend_ctrl.Result, error)

type BackendFuncController struct {
	backend_ctrl.BaseController
	Func BackendFunc
}

func (b *BackendFuncController) Run(ctx context.Context, request *backend_ctrl.Request) (backend_ctrl.Result, error) {
	return b.Func(ctx, request)
}

// AsyncResource creates an HTTP handler that can be used to test asynchronous resource lifecycle operations.
func AsyncResource(t *testing.T, ts *testserver.TestServer, rootScope string, put BackendFunc, delete BackendFunc) func(w http.ResponseWriter, r *http.Request) {
	rootScope = strings.ToLower(rootScope)

	ctx := testcontext.New(t)
	r := chi.NewRouter()
	r.Use(servicecontext.ARMRequestCtx("", v1.LocationGlobal), middleware.LowercaseURLPath)

	resourceType := "System.Test/testResources"

	queueClient, err := ts.Clients.QueueProvider.GetClient(ctx)
	require.NoError(t, err)

	statusManager := statusmanager.New(ts.Clients.StorageProvider, queueClient, v1.LocationGlobal)

	backendOpts := backend_ctrl.Options{
		DataProvider: ts.Clients.StorageProvider,
	}

	registry := worker.NewControllerRegistry(ts.Clients.StorageProvider)
	err = registry.Register(ctx, resourceType, v1.OperationPut, func(opts backend_ctrl.Options) (backend_ctrl.Controller, error) {
		return &BackendFuncController{BaseController: backend_ctrl.NewBaseAsyncController(opts), Func: put}, nil
	}, backendOpts)
	require.NoError(t, err)

	err = registry.Register(ctx, resourceType, v1.OperationDelete, func(opts backend_ctrl.Options) (backend_ctrl.Controller, error) {
		return &BackendFuncController{BaseController: backend_ctrl.NewBaseAsyncController(opts), Func: delete}, nil
	}, backendOpts)
	require.NoError(t, err)

	workerContext, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	w := worker.New(worker.Options{}, statusManager, queueClient, registry)
	go func() {
		err = w.Start(workerContext)
		require.NoError(t, err)
	}()

	frontendOpts := frontend_ctrl.Options{
		DataProvider:  ts.Clients.StorageProvider,
		StatusManager: statusManager,
	}

	err = server.ConfigureDefaultHandlers(ctx, r, rootScope, false, "System.Test", nil, frontendOpts)
	require.NoError(t, err)

	rootScopeRouter := server.NewSubrouter(r, rootScope)
	testResourceCollectionRouter := server.NewSubrouter(rootScopeRouter, "/providers/system.test/testresources")
	testResourceSingleRouter := server.NewSubrouter(rootScopeRouter, "/providers/system.test/testresources/{testResourceName}")

	resourceOptions := frontend_ctrl.ResourceOptions[TestResourceDatamodel]{
		RequestConverter:  TestResourceDataModelFromVersioned,
		ResponseConverter: TestResourceDataModelToVersioned,
	}

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: testResourceCollectionRouter,
			ResourceType: resourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt, resourceOptions)
			},
		},
		{
			ParentRouter: testResourceSingleRouter,
			ResourceType: resourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt, resourceOptions)
			},
		},
	}

	for _, h := range handlerOptions {
		err := server.RegisterHandler(ctx, h, frontendOpts)
		require.NoError(t, err)
	}

	return r.ServeHTTP
}
