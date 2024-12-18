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
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	backendctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var handlerTests = []rpctest.HandlerTestSpec{
	// applications.compute/virtualMachines
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.compute/virtualmachines",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: "ACTIONSTART"},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0/start",
		Method:        http.MethodPost,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: "ACTIONSTOP"},
		Path:          "/resourcegroups/testrg/providers/applications.compute/virtualmachines/vm0/stop",
		Method:        http.MethodPost,
	},
	// applications.compute/containers
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.compute/containers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0",
		Method:        http.MethodDelete,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers", Method: "ACTIONGETRESOURCE"},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/getresource",
		Method:        http.MethodPost,
	},
	// applications.compute/containers/secrets
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/containers/secrets", Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/secrets",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers/secrets", Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/secrets/secret0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers/secrets", Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/secrets/secret0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers/secrets", Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/secrets/secret0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/containers/secrets", Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.compute/containers/container0/secrets/secret0",
		Method:        http.MethodDelete,
	},
	// applications.compute/webassemblies
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationPlaneScopeList},
		Path:          "/providers/applications.compute/webassemblies",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationList},
		Path:          "/resourcegroups/testrg/providers/applications.compute/webassemblies",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationGet},
		Path:          "/resourcegroups/testrg/providers/applications.compute/webassemblies/wasm0",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationPut},
		Path:          "/resourcegroups/testrg/providers/applications.compute/webassemblies/wasm0",
		Method:        http.MethodPut,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationPatch},
		Path:          "/resourcegroups/testrg/providers/applications.compute/webassemblies/wasm0",
		Method:        http.MethodPatch,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/webassemblies", Method: v1.OperationDelete},
		Path:          "/resourcegroups/testrg/providers/applications.compute/webassemblies/wasm0",
		Method:        http.MethodDelete,
	},
}

var defaultHandlerTests = []rpctest.HandlerTestSpec{
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/operations", Method: v1.OperationGet},
		Path:          "/providers/applications.compute/operations",
		Method:        http.MethodGet,
	},
	// default operations
	{
		OperationType: v1.OperationType{Type: "Applications.Compute/operationStatuses", Method: v1.OperationGet},
		Path:          "/providers/applications.compute/locations/global/operationstatuses/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	}, {
		OperationType: v1.OperationType{Type: "Applications.Compute/operationResults", Method: v1.OperationGet},
		Path:          "/providers/applications.compute/locations/global/operationresults/00000000-0000-0000-0000-000000000000",
		Method:        http.MethodGet,
	},
}

func TestApplyAPIHandlers(t *testing.T) {
	runTests := func(t *testing.T, testSpecs []rpctest.HandlerTestSpec, b *Builder) {
		rpctest.AssertRequests(t, testSpecs, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
			r := chi.NewRouter()

			options := apictrl.Options{
				Address:        "localhost:8080",
				PathBase:       "/api.ucp.dev",
				DatabaseClient: inmemory.NewClient(),
				StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
			}

			return r, b.ApplyAPIHandlers(ctx, r, options)
		})
	}

	t.Run("custom handlers", func(t *testing.T) {
		ns := newTestNamespace(t)
		builder := ns.GenerateBuilder()
		runTests(t, handlerTests, &builder)
	})

	t.Run("default handlers", func(t *testing.T) {
		ns := newTestNamespace(t)
		builder := ns.GenerateBuilder()
		ns.SetAvailableOperations([]v1.Operation{
			{
				Name: "Applications.Compute/operations/read",
				Display: &v1.OperationDisplayProperties{
					Provider:    "Applications.Compute",
					Resource:    "operations",
					Operation:   "Get operations",
					Description: "Get the list of operations",
				},
				IsDataAction: false,
			},
		})
		runTests(t, defaultHandlerTests, &builder)
	})
}

func TestApplyAPIHandlers_AvailableOperations(t *testing.T) {
	ns := newTestNamespace(t)

	ns.SetAvailableOperations([]v1.Operation{
		{
			Name: "Applications.Compute/operations/read",
			Display: &v1.OperationDisplayProperties{
				Provider:    "Applications.Compute",
				Resource:    "operations",
				Operation:   "Get operations",
				Description: "Get the list of operations",
			},
			IsDataAction: false,
		},
	})

	builder := ns.GenerateBuilder()
	rpctest.AssertRequests(t, handlerTests, "/api.ucp.dev", "/planes/radius/local", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		options := apictrl.Options{
			Address:        "localhost:8080",
			PathBase:       "/api.ucp.dev",
			DatabaseClient: inmemory.NewClient(),
			StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
		}
		return r, builder.ApplyAPIHandlers(ctx, r, options)
	})
}

func TestApplyAsyncHandler(t *testing.T) {
	ns := newTestNamespace(t)
	builder := ns.GenerateBuilder()
	registry := worker.NewControllerRegistry()
	ctx := testcontext.New(t)

	options := backendctrl.Options{
		DatabaseClient: inmemory.NewClient(),
	}

	err := builder.ApplyAsyncHandler(ctx, registry, options)
	require.NoError(t, err)

	expectedOperations := []v1.OperationType{
		{Type: "Applications.Compute/virtualMachines", Method: v1.OperationPut},
		{Type: "Applications.Compute/virtualMachines", Method: v1.OperationPatch},
		{Type: "Applications.Compute/virtualMachines", Method: "ACTIONSTART"},
		{Type: "Applications.Compute/virtualMachines/disks", Method: v1.OperationPut},
		{Type: "Applications.Compute/virtualMachines/disks", Method: v1.OperationPatch},
		{Type: "Applications.Compute/webAssemblies", Method: v1.OperationPut},
		{Type: "Applications.Compute/webAssemblies", Method: v1.OperationPatch},
	}

	for _, op := range expectedOperations {
		jobCtrl, err := registry.Get(op)
		require.NoError(t, err)
		require.NotNil(t, jobCtrl)
	}
}
