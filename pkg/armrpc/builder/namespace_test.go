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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	asyncctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

type testAPIController struct {
	apictrl.Operation[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]
}

func (e *testAPIController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	return nil, nil
}

type testAsyncController struct {
}

func (c *testAsyncController) Run(ctx context.Context, request *asyncctrl.Request) (asyncctrl.Result, error) {
	return asyncctrl.Result{}, nil
}

func (c *testAsyncController) StorageClient() store.StorageClient {
	return nil
}

func newTestController(opts apictrl.Options) (apictrl.Controller, error) {
	return &testAPIController{
		apictrl.NewOperation(opts,
			apictrl.ResourceOptions[rpctest.TestResourceDataModel]{
				RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
				ResponseConverter: rpctest.TestResourceDataModelToVersioned,
			},
		),
	}, nil
}

func newTestNamespace(t *testing.T) *Namespace {
	ns := NewNamespace("Applications.Compute")
	require.Equal(t, NamespaceResourceKind, ns.Kind)
	require.Equal(t, "Applications.Compute", ns.Name)

	asyncFunc := func(opts asyncctrl.Options) (asyncctrl.Controller, error) {
		return &testAsyncController{}, nil
	}

	// register virtualMachines resource
	vmResource := ns.AddResource("virtualMachines", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		ResourceParamName: "virtualMachineName",

		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
		Custom: map[string]Operation[rpctest.TestResourceDataModel]{
			"start": {
				APIController:      newTestController,
				AsyncJobController: asyncFunc,
			},
			"stop": {
				APIController: newTestController,
			},
		},
	})

	require.NotNil(t, vmResource)

	// register virtualMachines/disks child resource
	_ = vmResource.AddResource("disks", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
		Custom: map[string]Operation[rpctest.TestResourceDataModel]{
			"replace": {
				APIController: newTestController,
			},
		},
	})

	// register virtualMachines/networks child resource
	_ = vmResource.AddResource("networks", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
		Custom: map[string]Operation[rpctest.TestResourceDataModel]{
			"connect": {
				APIController: newTestController,
			},
		},
	})

	// register containers resource
	containerResource := ns.AddResource("containers", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
		Custom: map[string]Operation[rpctest.TestResourceDataModel]{
			"getresource": {
				APIController: newTestController,
			},
		},
	})

	require.NotNil(t, containerResource)

	// register containers/secrets child resource
	_ = containerResource.AddResource("secrets", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController: newTestController,
		},
	})

	// register webAssemblies resource
	wasmResource := ns.AddResource("webAssemblies", &ResourceOption[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]{
		ResourceParamName: "webAssemblyName",
		RequestConverter:  rpctest.TestResourceDataModelFromVersioned,
		ResponseConverter: rpctest.TestResourceDataModelToVersioned,

		Put: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
		Patch: Operation[rpctest.TestResourceDataModel]{
			APIController:      newTestController,
			AsyncJobController: asyncFunc,
		},
	})

	require.NotNil(t, wasmResource)

	return ns
}

func TestNamespaceBuild(t *testing.T) {
	ns := newTestNamespace(t)
	builders := ns.GenerateBuilder()
	require.NotNil(t, builders)

	builderTests := []struct {
		resourceType        string
		resourceNamePattern string
		path                string
		method              v1.OperationMethod
		found               bool
	}{
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines",
			path:                "",
			method:              "LISTPLANESCOPE",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "",
			method:              "DELETE",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "/start",
			method:              "ACTIONSTART",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}",
			path:                "/stop",
			method:              "ACTIONSTOP",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks/{networkName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks/{networkName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks/{networkName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks/{networkName}",
			path:                "",
			method:              "DELETE",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/networks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/networks/{networkName}",
			path:                "/connect",
			method:              "ACTIONCONNECT",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks/{diskName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks/{diskName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks/{diskName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks/{diskName}",
			path:                "",
			method:              "DELETE",
		},
		{
			resourceType:        "Applications.Compute/virtualMachines/disks",
			resourceNamePattern: "applications.compute/virtualmachines/{virtualMachineName}/disks/{diskName}",
			path:                "/replace",
			method:              "ACTIONREPLACE",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers",
			path:                "",
			method:              "LISTPLANESCOPE",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers/{containerName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers/{containerName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers/{containerName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers/{containerName}",
			path:                "",
			method:              "DELETE",
		},
		{
			resourceType:        "Applications.Compute/containers",
			resourceNamePattern: "applications.compute/containers/{containerName}",
			path:                "/getresource",
			method:              "ACTIONGETRESOURCE",
		},
		{
			resourceType:        "Applications.Compute/containers/secrets",
			resourceNamePattern: "applications.compute/containers/{containerName}/secrets",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/containers/secrets",
			resourceNamePattern: "applications.compute/containers/{containerName}/secrets/{secretName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/containers/secrets",
			resourceNamePattern: "applications.compute/containers/{containerName}/secrets/{secretName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/containers/secrets",
			resourceNamePattern: "applications.compute/containers/{containerName}/secrets/{secretName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/containers/secrets",
			resourceNamePattern: "applications.compute/containers/{containerName}/secrets/{secretName}",
			path:                "",
			method:              "DELETE",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies",
			path:                "",
			method:              "LISTPLANESCOPE",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies",
			path:                "",
			method:              "LIST",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies/{webAssemblyName}",
			path:                "",
			method:              "GET",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies/{webAssemblyName}",
			path:                "",
			method:              "PUT",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies/{webAssemblyName}",
			path:                "",
			method:              "PATCH",
		},
		{
			resourceType:        "Applications.Compute/webAssemblies",
			resourceNamePattern: "applications.compute/webassemblies/{webAssemblyName}",
			path:                "",
			method:              "DELETE",
		},
	}

	for _, b := range builders.registrations {
		for i, bt := range builderTests {
			if bt.resourceType == b.ResourceType && bt.resourceNamePattern == b.ResourceNamePattern && bt.path == b.Path && bt.method == b.Method {
				builderTests[i].found = true
			}
		}
	}

	for _, bt := range builderTests {
		require.True(t, bt.found, "resource not found: %s %s %s %s", bt.resourceType, bt.resourceNamePattern, bt.path, bt.method)
	}
}
