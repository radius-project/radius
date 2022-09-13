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
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	awsproxy_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
	kubernetes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/kubernetes"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	resourcegroups_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"
)

// TODO: Use variables and construct the path as we add more APIs.
const (
	planeCollectionPath       = "/planes"
	awsPlaneType              = "/planes/aws"
	planeItemPath             = "/planes/{PlaneType}/{PlaneID}"
	planeCollectionByType     = "/planes/{PlaneType}"
	awsOperationResultsPath   = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/locations/{Location}/operationResults/{operationID}"
	awsOperationStatusesPath  = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/locations/{Location}/operationStatuses/{operationID}"
	awsResourceCollectionPath = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/{ResourceType}"
	awsResourcePath           = "/{AWSPlaneName}/accounts/{AccountID}/regions/{Region}/providers/{Provider}/{ResourceType}/{ResourceName}"
)

var resourceGroupCollectionPath = fmt.Sprintf("%s/%s", planeItemPath, "resource{[gG]}roups")
var resourceGroupItemPath = fmt.Sprintf("%s/%s", resourceGroupCollectionPath, "{ResourceGroup}")

// Register registers the routes for UCP
func Register(ctx context.Context, router *mux.Router, ctrlOpts ctrl.Options) error {
	baseURL := ctrlOpts.BasePath

	handlerOptions := []ctrl.HandlerOptions{}

	// If we're in Kubernetes we have some required routes to implement.
	if baseURL != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.
		handlerOptions = append(handlerOptions, []ctrl.HandlerOptions{
			{
				ParentRouter:   router.Path("/openapi/v2").Subrouter(),
				Method:         v1.OperationGet,
				HandlerFactory: kubernetes_ctrl.NewOpenAPIv2Doc,
			},
			{
				ParentRouter:   router.Path(baseURL).Subrouter(),
				Method:         v1.OperationGet,
				HandlerFactory: kubernetes_ctrl.NewDiscoveryDoc,
			},
		}...)

	}
	logger := ucplog.GetLogger(ctx)
	logger.Info(fmt.Sprintf("UCP base path: %s", baseURL))

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, baseURL+planeCollectionPath)
	if err != nil {
		return err
	}

	rootScopeRouter := router.PathPrefix(baseURL + planeCollectionPath).Subrouter()
	rootScopeRouter.Use(validator.APIValidatorUCP(specLoader))
	ctrl.ConfigureDefaultHandlers(router, ctrl.Options{
		BasePath: baseURL,
	})

	planeCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeCollectionPath)).Subrouter()
	planeCollectionByTypeSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeCollectionByType)).Subrouter()
	planeSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, planeItemPath)).Subrouter()

	resourceGroupCollectionSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, resourceGroupCollectionPath)).Subrouter()
	resourceGroupSubRouter := router.Path(fmt.Sprintf("%s%s", baseURL, resourceGroupItemPath)).Subrouter()

	awsResourcesSubRouter := router.PathPrefix(fmt.Sprintf("%s%s", baseURL, awsPlaneType)).Subrouter()
	awsResourceCollectionSubRouter := awsResourcesSubRouter.Path(fmt.Sprintf("%s", awsResourceCollectionPath)).Subrouter()
	awsSingleResourceSubRouter := awsResourcesSubRouter.Path(fmt.Sprintf("%s", awsResourcePath)).Subrouter()
	awsOperationStatusesSubRouter := awsResourcesSubRouter.PathPrefix(awsOperationStatusesPath).Subrouter()
	awsOperationResultsSubRouter := awsResourcesSubRouter.PathPrefix(awsOperationResultsPath).Subrouter()

	handlerOptions = append(handlerOptions, []ctrl.HandlerOptions{
		// Planes resource handler registration.
		{
			ParentRouter:   planeCollectionSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: planes_ctrl.NewListPlanes,
		},
		{
			ParentRouter:   planeCollectionByTypeSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: planes_ctrl.NewListPlanes,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: planes_ctrl.NewGetPlane,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationPut,
			HandlerFactory: planes_ctrl.NewCreateOrUpdatePlane,
		},
		{
			ParentRouter:   planeSubRouter,
			Method:         v1.OperationDelete,
			HandlerFactory: planes_ctrl.NewDeletePlane,
		},
		// Resource group handler registration
		{
			ParentRouter:   resourceGroupCollectionSubRouter,
			Method:         v1.OperationList,
			HandlerFactory: resourcegroups_ctrl.NewListResourceGroups,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: resourcegroups_ctrl.NewGetResourceGroup,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationPut,
			HandlerFactory: resourcegroups_ctrl.NewCreateOrUpdateResourceGroup,
		},
		{
			ParentRouter:   resourceGroupSubRouter,
			Method:         v1.OperationDelete,
			HandlerFactory: resourcegroups_ctrl.NewDeleteResourceGroup,
		},

		// AWS Plane handlers
		{
			ParentRouter:   awsOperationResultsSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: awsproxy_ctrl.NewGetAWSOperationResults,
		},
		{
			ParentRouter:   awsOperationStatusesSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: awsproxy_ctrl.NewGetAWSOperationStatuses,
		},
		{
			ParentRouter:   awsResourceCollectionSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: awsproxy_ctrl.NewListAWSResources,
		},
		{
			ParentRouter:   awsSingleResourceSubRouter,
			Method:         v1.OperationPut,
			HandlerFactory: awsproxy_ctrl.NewCreateOrUpdateAWSResource,
		},
		{
			ParentRouter:   awsSingleResourceSubRouter,
			Method:         v1.OperationDelete,
			HandlerFactory: awsproxy_ctrl.NewDeleteAWSResource,
		},
		{
			ParentRouter:   awsSingleResourceSubRouter,
			Method:         v1.OperationGet,
			HandlerFactory: awsproxy_ctrl.NewGetAWSResource,
		},

		// Proxy request should take the least priority in routing and should therefore be last
		{
			ParentRouter:   router,
			Path:           fmt.Sprintf("%s%s", baseURL, planeItemPath),
			HandlerFactory: planes_ctrl.NewProxyPlane,
		},
	}...)

	for _, h := range handlerOptions {
		if err := ctrl.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
