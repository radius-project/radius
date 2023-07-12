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

/*

Package server provides HTTP server and router utilities for enabling the Radius
resource provider. It utilizes the go-chi router (https://github.com/go-chi/chi)
to handle ARM RPC requests (https://github.com/Azure/azure-resource-manager-rpc/blob/master/README.md).

## Defining a New Resource Type in the Radius Resource Provider

To model a resource type, we need to define the REST API paths. For example,
we need to define the following API routes to model a resource type "foo":

    GET    /planes/{planeType}/{planeName}/providers/Applications.Core/foo
    GET    /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo
    GET    /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}
    PUT    /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}
    PATCH  /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}
    DELETE /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}

To define these routes, we first need to create subrouters and then register
handlers for each method. It is recommended to use the full path name in
server.NewSubrouter to avoid conflicts with existing routes. Each subrouter
must have a validator middleware to enable OpenAPI validation.

Here's an example of creating subrouters:

    // Subrouter for /planes/{planeType}/{planeName}/providers/Applications.Core/foo
    fooPlaneRouter := server.NewSubrouter(r, "/planes/{planeType}/{planeName}/providers/applications.core/foo", validator)

    // Subrouter for /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo
    fooResourceGroupRouter := server.NewSubrouter(r, "/planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/applications.core/foo", validator)

    // Subrouter for /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}
    fooResourceRouter := server.NewSubrouter(r, "/planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/applications.core/foo/{fooName}", validator)

To register the handlers, we need to provide the necessary options and controller factories. Here's an example:

    handlerOptions := []server.HandlerOptions{
        {
            ParentRouter: fooPlaneRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationList,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            },
        },
        {
            ParentRouter: fooResourceGroupRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationList,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            }
        },
        {
            ParentRouter: fooResourceRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationGet,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            }
        },
        {
            ParentRouter: fooResourceRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationPut,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            }
        },
        {
            ParentRouter: fooResourceRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationPatch,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            }
        },
        {
            ParentRouter: fooResourceRouter,
            ResourceType: "Applications.Core/foo",
            Method:       v1.OperationDelete,
            ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
                ...
            }
        },
    }

    for _, h := range handlerOptions {
        if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
            return err
        }
    }


# Muti-level subrouters and middleware invocation

The following code demonstrates the use of multi-level subrouters to define
the "foo" resource API. Each router has its own middleware:

    planeScopeRouter := server.NewSubrouter(r, "/planes/{planeType}/{planeName}", middleware1)
    fooResourceGroupRouter := server.NewSubrouter(planeScopeRouter, "/resourceGroups/{resourceGroupName}/providers/applications.core/foo", middleware2)
    fooResourceRouter := server.NewSubrouter(fooResourceGroupRouter, "/{fooName}", middleware3)

When the router handles the request:

    GET /planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}/providers/Applications.Core/foo/{fooName}, the middleware will be called in the following order:

The middleware will be called in the following order:

/planes/{planeType}/{planeName}/*   <--- middleware1
                               /resourceGroups/{resourceGroupName}/providers/applications.core/foo/*   <--- middleware2
                                                                                                    /{fooName}/  <--- middleware3

These three-level routers call their middleware in the order of middleware1,
middleware2, and middleware3. Even if the entire request path is unmatched
middleware1 can still be called if '/planes/{planeType}/{planeName}/*' matches.
Therefore, if you want to call middleware for the fully matched path, you need
to add the middleware to the last router. Otherwise, the middleware should always
skip processing the request if its path has a catch-all route pattern (/*).

*/
