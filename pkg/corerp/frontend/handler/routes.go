// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"
)

const (
	APIVersionParam = "api-version"
)

type ControllerFunc func(store.StorageClient, deployment.DeploymentProcessor) (controller.ControllerInterface, error)

type handlerParams struct {
	parent       *mux.Router
	resourcetype string
	method       string
	fn           ControllerFunc
}

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(ctx context.Context, sp *dataprovider.StorageProvider, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) error {
	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	providerRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Tenant level API routes.
	tenantLevelPath := pathBase + "/providers/applications.core"
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	operationsRouter := router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Resource Group level API routes.
	resourceGroupLevelPath := pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core"

	// Adds environment resource type routes
	envRTSubrouter := router.PathPrefix(resourceGroupLevelPath+"/environments").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.Path("/{environment}").Subrouter()

	handlers := []handlerParams{
		// Provider handler registration.
		{providerRouter, provider_ctrl.ResourceTypeName, http.MethodPut, provider_ctrl.NewCreateOrUpdateSubscription},
		{operationsRouter, provider_ctrl.ResourceTypeName, http.MethodGet, provider_ctrl.NewGetOperations},
		// Environments resource handler registration.
		{envRTSubrouter.Path("/").Subrouter(), env_ctrl.ResourceTypeName, http.MethodGet, env_ctrl.NewListEnvironments},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodGet, env_ctrl.NewGetEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPut, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPatch, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodDelete, env_ctrl.NewDeleteEnvironment},
	}

	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.fn); err != nil {
			return err
		}
	}

	return nil
}

func registerHandler(ctx context.Context, sp *dataprovider.StorageProvider, parent *mux.Router, resourcetype string, method string, CreateController ControllerFunc) error {
	sc, err := sp.GetStorageClient(ctx, resourcetype)
	if err != nil {
		return err
	}

	ctrl, err := CreateController(sc, nil)
	if err != nil {
		return err
	}

	fn := func(w http.ResponseWriter, req *http.Request) {
		hctx := req.Context()

		response, err := ctrl.Run(hctx, req)
		if err != nil {
			internalServerError(hctx, w, req, err)
			return
		}
		err = response.Apply(hctx, w, req)
		if err != nil {
			internalServerError(hctx, w, req, err)
			return
		}
	}
	parent.Methods(method).HandlerFunc(fn)
	return nil
}

// Responds with an HTTP 500
func internalServerError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := radlogger.GetLogger(ctx)
	logger.Error(err, "unhandled error")

	// Try to use the ARM format to send back the error info
	body := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: err.Error(),
		},
	}

	response := rest.NewInternalServerErrorARMResponse(body)
	err = response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}
