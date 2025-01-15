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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	azure_credential_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/credentials/azure"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	planeCollectionPath = "/planes/azure"
	planeResourcePath   = "/planes/azure/{planeName}"

	credentialResourcePath   = planeResourcePath + "/providers/System.Azure/credentials/{credentialName}"
	credentialCollectionPath = planeResourcePath + "/providers/System.Azure/credentials"

	// OperationTypeUCPAzureProxy is the operation type for proxying Azure API calls.
	OperationTypeUCPAzureProxy = "UCPAZUREPROXY"
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	secretClient, err := m.options.SecretProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	baseRouter := server.NewSubrouter(m.router, m.options.Config.Server.PathBase+"/")

	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         m.options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	planeResourceOptions := controller.ResourceOptions[datamodel.AzurePlane]{
		RequestConverter:  converter.AzurePlaneDataModelFromVersioned,
		ResponseConverter: converter.AzurePlaneDataModelToVersioned,
	}

	// URLs for lifecycle of planes
	planeCollectionRouter := server.NewSubrouter(baseRouter, planeCollectionPath, apiValidator)
	planeResourceRouter := server.NewSubrouter(baseRouter, planeResourcePath, apiValidator)

	credentialCollectionRouter := server.NewSubrouter(baseRouter, credentialCollectionPath, apiValidator)
	credentialResourceRouter := server.NewSubrouter(baseRouter, credentialResourcePath, apiValidator)

	handlerOptions := []server.HandlerOptions{
		{
			// This is a scope query so we can't use the default operation.
			ParentRouter:  planeCollectionRouter,
			Method:        v1.OperationList,
			ResourceType:  datamodel.AzurePlaneResourceType,
			OperationType: &v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationList},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return &planes_ctrl.ListPlanesByType[*datamodel.AzurePlane, datamodel.AzurePlane]{
					Operation: controller.NewOperation(opts, planeResourceOptions),
				}, nil
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationGet,
			ResourceType:  datamodel.AzurePlaneResourceType,
			OperationType: &v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationGet},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationPut,
			ResourceType:  datamodel.AzurePlaneResourceType,
			OperationType: &v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationPut},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationDelete,
			ResourceType:  datamodel.AzurePlaneResourceType,
			OperationType: &v1.OperationType{Type: datamodel.AzurePlaneResourceType, Method: v1.OperationDelete},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter: credentialCollectionRouter,
			ResourceType: v20231001preview.AzureCredentialType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewListResources(opt,
					controller.ResourceOptions[datamodel.AzureCredential]{
						RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
						ResponseConverter: converter.AzureCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			ResourceType: v20231001preview.AzureCredentialType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					controller.ResourceOptions[datamodel.AzureCredential]{
						RequestConverter:  converter.AzureCredentialDataModelFromVersioned,
						ResponseConverter: converter.AzureCredentialDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationPut,
			ResourceType: v20231001preview.AzureCredentialType,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return azure_credential_ctrl.NewCreateOrUpdateAzureCredential(opt, secretClient)
			},
		},
		{
			ParentRouter: credentialResourceRouter,
			Method:       v1.OperationDelete,
			ResourceType: v20231001preview.AzureCredentialType,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return azure_credential_ctrl.NewDeleteAzureCredential(opt, secretClient)
			},
		},

		// Chi router uses radix tree so that it doesn't linear search the matched one. So, to catch all requests,
		// we need to use CatchAllPath(/*) at the above matched routes path in chi router.
		//
		// Note that the API validation is not applied for CatchAllPath(/*).
		{
			// Method deliberately omitted. This is a catch-all route for proxying.
			ParentRouter:      planeResourceRouter,
			Path:              server.CatchAllPath,
			OperationType:     &v1.OperationType{Type: OperationTypeUCPAzureProxy, Method: v1.OperationProxy},
			ResourceType:      OperationTypeUCPAzureProxy,
			ControllerFactory: planes_ctrl.NewProxyController,
		},
	}

	databaseClient, err := m.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	ctrlOpts := controller.Options{
		Address:        m.options.Config.Server.Address(),
		DatabaseClient: databaseClient,
		PathBase:       m.options.Config.Server.PathBase,
		StatusManager:  m.options.StatusManager,

		KubeClient:   nil, // Unused by Azure module
		ResourceType: "",  // Set dynamically
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}
