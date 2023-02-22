// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"context"
	"fmt"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ucp_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
	awsproxy_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
	aws_credential_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials/aws"
	azure_credential_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/credentials/azure"
	kubernetes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/kubernetes"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	resourcegroups_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"
)

// TODO: Use variables and construct the path as we add more APIs.
const (
	planeCollectionPath           = "/planes"
	awsPlaneType                  = "/planes/aws"
	planeItemPath                 = "/planes/{planeType}/{planeName}"
	planeCollectionByType         = "/planes/{planeType}"
	awsOperationResultsPath       = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/locations/{Location}/operationResults/{operationID}"
	awsOperationStatusesPath      = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/locations/{Location}/operationStatuses/{operationID}"
	awsResourceCollectionPath     = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/{ResourceType}"
	awsResourcePath               = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/{ResourceType}/{ResourceName}"
	putPath                       = "put"
	getPath                       = "get"
	deletePath                    = "delete"
	azureCredentialCollectionPath = "/planes/azure/{planeName}/providers/{Provider}/{ResourceType}"
	azureCredentialResourcePath   = "/planes/azure/{planeName}/providers/{Provider}/{ResourceType}/{ResourceName}"
	awsCredentialCollectionPath   = "/planes/aws/{planeName}/providers/{Provider}/{ResourceType}"
	awsCredentialResourcePath     = "/planes/aws/{planeName}/providers/{Provider}/{ResourceType}/{ResourceName}"
)

// Register registers the routes for UCP
func Register(ctx context.Context, router *mux.Router, ctrlOpts ctrl.Options, awsOpts awsproxy.AWSOptions) error {
	baseURL := ctrlOpts.BasePath

	handlerOptions := []ucp_ctrl.HandlerOptions{}

	// If we're in Kubernetes we have some required routes to implement.
	if baseURL != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.

		handlerOptions = append(handlerOptions, []ucp_ctrl.HandlerOptions{
			{
				server.HandlerOptions{
					ParentRouter:   router.Path("/openapi/v2").Subrouter(),
					Method:         v1.OperationGet,
					HandlerFactory: kubernetes_ctrl.NewOpenAPIv2Doc,
				}, nil,
			},
			{
				server.HandlerOptions{
					ParentRouter:   router.Path(baseURL).Subrouter(),
					Method:         v1.OperationGet,
					HandlerFactory: kubernetes_ctrl.NewDiscoveryDoc,
				}, nil,
			},
		}...)

	}

	// No default handlers configured at this point.

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Registering routes with base path: %s", baseURL))

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, baseURL, "")
	if err != nil {
		return err
	}

	rootScopeRouter := router.PathPrefix(baseURL).Subrouter()
	rootScopeRouter.Use(validator.APIValidatorUCP(specLoader))

	planeCollectionSubRouter := rootScopeRouter.Path(planeCollectionPath).Subrouter()
	planeCollectionByTypeSubRouter := rootScopeRouter.Path(planeCollectionByType).Subrouter()
	planeSubRouter := rootScopeRouter.Path(planeItemPath).Subrouter()

	var resourceGroupCollectionPath = fmt.Sprintf("%s/%s", planeItemPath, "resourcegroups")
	var resourceGroupItemPath = fmt.Sprintf("%s/%s", resourceGroupCollectionPath, "{resourceGroupName}")
	resourceGroupCollectionSubRouter := rootScopeRouter.Path(resourceGroupCollectionPath).Subrouter()
	resourceGroupSubRouter := rootScopeRouter.Path(resourceGroupItemPath).Subrouter()

	awsResourcesSubRouter := router.PathPrefix(fmt.Sprintf("%s%s", baseURL, awsPlaneType)).Subrouter()
	awsResourceCollectionSubRouter := awsResourcesSubRouter.Path(awsResourceCollectionPath).Subrouter()
	awsSingleResourceSubRouter := awsResourcesSubRouter.Path(awsResourcePath).Subrouter()
	awsOperationStatusesSubRouter := awsResourcesSubRouter.PathPrefix(awsOperationStatusesPath).Subrouter()
	awsOperationResultsSubRouter := awsResourcesSubRouter.PathPrefix(awsOperationResultsPath).Subrouter()
	awsPutResourceSubRouter := awsResourcesSubRouter.Path(fmt.Sprintf("%s/:%s", awsResourceCollectionPath, putPath)).Subrouter()
	awsGetResourceSubRouter := awsResourcesSubRouter.Path(fmt.Sprintf("%s/:%s", awsResourceCollectionPath, getPath)).Subrouter()
	awsDeleteResourceSubRouter := awsResourcesSubRouter.Path(fmt.Sprintf("%s/:%s", awsResourceCollectionPath, deletePath)).Subrouter()

	azureCredentialCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, azureCredentialCollectionPath)).Subrouter()
	azureCredentialResourceSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, azureCredentialResourcePath)).Subrouter()
	awsCredentialCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, awsCredentialCollectionPath)).Subrouter()
	awsCredentialResourceSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, awsCredentialResourcePath)).Subrouter()

	handlerOptions = append(handlerOptions, []ucp_ctrl.HandlerOptions{
		// Planes resource handler registration.
		{
			server.HandlerOptions{
				ParentRouter:   planeCollectionSubRouter,
				Method:         v1.OperationList,
				HandlerFactory: planes_ctrl.NewListPlanes,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   planeCollectionByTypeSubRouter,
				Method:         v1.OperationList,
				HandlerFactory: planes_ctrl.NewListPlanesByType,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   planeSubRouter,
				Method:         v1.OperationGet,
				HandlerFactory: planes_ctrl.NewGetPlane,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   planeSubRouter,
				Method:         v1.OperationPut,
				HandlerFactory: planes_ctrl.NewCreateOrUpdatePlane,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   planeSubRouter,
				Method:         v1.OperationDelete,
				HandlerFactory: planes_ctrl.NewDeletePlane,
			}, nil,
		},
		// Resource group handler registration
		{
			server.HandlerOptions{
				ParentRouter:   resourceGroupCollectionSubRouter,
				Method:         v1.OperationList,
				HandlerFactory: resourcegroups_ctrl.NewListResourceGroups,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   resourceGroupSubRouter,
				Method:         v1.OperationGet,
				HandlerFactory: resourcegroups_ctrl.NewGetResourceGroup,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   resourceGroupSubRouter,
				Method:         v1.OperationPut,
				HandlerFactory: resourcegroups_ctrl.NewCreateOrUpdateResourceGroup,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   resourceGroupSubRouter,
				Method:         v1.OperationDelete,
				HandlerFactory: resourcegroups_ctrl.NewDeleteResourceGroup,
			}, nil,
		},

		// AWS Plane handlers
		{
			server.HandlerOptions{
				ParentRouter: awsOperationResultsSubRouter,
				Method:       v1.OperationGet,
			},
			awsproxy_ctrl.NewGetAWSOperationResults,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsOperationStatusesSubRouter,
				Method:       v1.OperationGet,
			},
			awsproxy_ctrl.NewGetAWSOperationStatuses,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsResourceCollectionSubRouter,
				Method:       v1.OperationGet,
			},
			awsproxy_ctrl.NewListAWSResources,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsSingleResourceSubRouter,
				Method:       v1.OperationPut,
			},
			awsproxy_ctrl.NewCreateOrUpdateAWSResource,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsSingleResourceSubRouter,
				Method:       v1.OperationDelete,
			},
			awsproxy_ctrl.NewDeleteAWSResource,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsSingleResourceSubRouter,
				Method:       v1.OperationGet,
			},
			awsproxy_ctrl.NewGetAWSResource,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsPutResourceSubRouter,
				Method:       v1.OperationPost,
			},
			awsproxy_ctrl.NewCreateOrUpdateAWSResourceWithPost,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsGetResourceSubRouter,
				Method:       v1.OperationPost,
			},
			awsproxy_ctrl.NewGetAWSResourceWithPost,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsDeleteResourceSubRouter,
				Method:       v1.OperationPost,
			},
			awsproxy_ctrl.NewDeleteAWSResourceWithPost,
		},

		// Azure Credential Handlers
		{
			server.HandlerOptions{
				ParentRouter: azureCredentialCollectionSubRouter,
				ResourceType: v20220901privatepreview.AzureCredentialType,
				Method:       v1.OperationList,
				HandlerFactory: func(opt ctrl.Options) (ctrl.Controller, error) {
					return defaultoperation.NewListResources(opt,
						ctrl.ResourceOptions[datamodel.Credential]{
							RequestConverter:  converter.CredentialDataModelFromVersioned,
							ResponseConverter: converter.CredentialDataModelToVersioned,
						},
					)
				},
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter: azureCredentialResourceSubRouter,
				ResourceType: v20220901privatepreview.AzureCredentialType,
				Method:       v1.OperationGet,
				HandlerFactory: func(opt ctrl.Options) (ctrl.Controller, error) {
					return defaultoperation.NewGetResource(opt,
						ctrl.ResourceOptions[datamodel.Credential]{
							RequestConverter:  converter.CredentialDataModelFromVersioned,
							ResponseConverter: converter.CredentialDataModelToVersioned,
						},
					)
				},
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   azureCredentialResourceSubRouter,
				Method:         v1.OperationPut,
				HandlerFactory: azure_credential_ctrl.NewCreateOrUpdateCredential,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   azureCredentialResourceSubRouter,
				Method:         v1.OperationDelete,
				HandlerFactory: azure_credential_ctrl.NewDeleteCredential,
			}, nil,
		},

		// AWS Credential Handlers
		{
			server.HandlerOptions{
				ParentRouter: awsCredentialCollectionSubRouter,
				ResourceType: v20220901privatepreview.AWSCredentialType,
				Method:       v1.OperationList,
				HandlerFactory: func(opt ctrl.Options) (ctrl.Controller, error) {
					return defaultoperation.NewListResources(opt,
						ctrl.ResourceOptions[datamodel.Credential]{
							RequestConverter:  converter.CredentialDataModelFromVersioned,
							ResponseConverter: converter.CredentialDataModelToVersioned,
						},
					)
				},
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter: awsCredentialResourceSubRouter,
				ResourceType: v20220901privatepreview.AWSCredentialType,
				Method:       v1.OperationGet,
				HandlerFactory: func(opt ctrl.Options) (ctrl.Controller, error) {
					return defaultoperation.NewGetResource(opt,
						ctrl.ResourceOptions[datamodel.Credential]{
							RequestConverter:  converter.CredentialDataModelFromVersioned,
							ResponseConverter: converter.CredentialDataModelToVersioned,
						},
					)
				},
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   awsCredentialResourceSubRouter,
				Method:         v1.OperationPut,
				HandlerFactory: aws_credential_ctrl.NewCreateOrUpdateCredential,
			}, nil,
		},
		{
			server.HandlerOptions{
				ParentRouter:   awsCredentialResourceSubRouter,
				Method:         v1.OperationDelete,
				HandlerFactory: aws_credential_ctrl.NewDeleteCredential,
			}, nil,
		},
		// Proxy request should take the least priority in routing and should therefore be last
		{
			// Note that the API validation is not applied to the router used for proxying
			server.HandlerOptions{
				ParentRouter:   router,
				Path:           fmt.Sprintf("%s%s", ctrlOpts.BasePath, planeItemPath),
				HandlerFactory: planes_ctrl.NewProxyPlane,
			}, nil,
		},
	}...)

	for _, h := range handlerOptions {
		if err := ucp_ctrl.RegisterHandler(ctx, h, ctrlOpts, awsOpts); err != nil {
			return err
		}
	}

	return nil
}
