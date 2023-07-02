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

package azure

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	azure_credential_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials/azure"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	"github.com/project-radius/radius/pkg/validator"
)

const (
	planeScope               = "/planes/azure/{planeName}"
	credentialResourcePath   = "/providers/System.Azure/credentials/{credentialName}"
	credentialCollectionPath = "/providers/System.Azure/credentials"

	// OperationTypeUCPAzureProxy is the operation type for proxying Azure API calls.
	OperationTypeUCPAzureProxy = "UCPAZUREPROXY"
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	secretClient, err := m.options.SecretProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	baseRouter := server.NewSubrouter(m.router, m.options.PathBase+planeScope)

	// URL for operations on System.Azure provider.
	apiValidator := validator.APIValidatorUCP(m.options.SpecLoader)

	credentialCollectionRouter := server.NewSubrouter(baseRouter, credentialCollectionPath, apiValidator)
	credentialResourceRouter := server.NewSubrouter(baseRouter, credentialResourcePath, apiValidator)

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: credentialCollectionRouter,
			ResourceType: v20220901privatepreview.AzureCredentialType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt armrpc_controller.Options) (armrpc_controller.Controller, error) {
				return defaultoperation.NewListResources(opt,
					armrpc_controller.ResourceOptions[datamodel.AzureCredential]{
						RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
						ResponseConverter: converter.AzureCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			ResourceType: v20220901privatepreview.AzureCredentialType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt armrpc_controller.Options) (armrpc_controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					armrpc_controller.ResourceOptions[datamodel.AzureCredential]{
						RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
						ResponseConverter: converter.AzureCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationPut,
			ResourceType: v20220901privatepreview.AzureCredentialType,
			ControllerFactory: func(opt armrpc_controller.Options) (armrpc_controller.Controller, error) {
				return azure_credential_ctrl.NewCreateOrUpdateAzureCredential(opt, secretClient)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationDelete,
			ResourceType: v20220901privatepreview.AzureCredentialType,
			ControllerFactory: func(opt armrpc_controller.Options) (armrpc_controller.Controller, error) {
				return azure_credential_ctrl.NewDeleteAzureCredential(opt, secretClient)
			},
		},

		// Proxy request should take the least priority in routing and should therefore be last
		//
		// Note that the API validation is not applied to the router used for proxying
		{
			// Method deliberately omitted. This is a catch-all route for proxying.
			ParentRouter:      baseRouter,
			Path:              "/*",
			OperationType:     &v1.OperationType{Type: OperationTypeUCPAzureProxy, Method: v1.OperationProxy},
			ControllerFactory: planes_ctrl.NewProxyPlane,
		},
	}

	ctrlOpts := armrpc_controller.Options{
		Address:      m.options.Address,
		PathBase:     m.options.PathBase,
		DataProvider: m.options.DataProvider,
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}
