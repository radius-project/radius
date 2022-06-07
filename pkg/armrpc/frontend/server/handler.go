// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	default_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/defaultcontroller"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	APIVersionParam = "api-version"
)

type ControllerFunc func(store.StorageClient, manager.StatusManager) (ctrl.Controller, error)

type HandlerOptions struct {
	ParentRouter   *mux.Router
	ResourceType   string
	Method         v1.OperationMethod
	HandlerFactory ControllerFunc
}

func RegisterHandler(ctx context.Context, sp dataprovider.DataStorageProvider, opts HandlerOptions) error {
	sc, err := sp.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}

	ctrl, err := opts.HandlerFactory(sc, nil)
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

	ot := v1.OperationType{Type: opts.ResourceType, Method: opts.Method}
	opts.ParentRouter.Methods(opts.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())
	return nil
}

func ConfigureDefaultHandlers(
	ctx context.Context,
	sp dataprovider.DataStorageProvider,
	rootRouter *mux.Router,
	scopeRouter *mux.Router,
	isSelfhosted bool,
	providerNamespace string,
	operationCtrlFactory ControllerFunc) error {
	providerNamespace = strings.ToLower(providerNamespace)
	rt := fmt.Sprintf("%s/provider", providerNamespace)

	if isSelfhosted {
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		err := RegisterHandler(ctx, sp, HandlerOptions{
			ParentRouter:   rootRouter.Path(fmt.Sprintf("/providers/%s/operations", providerNamespace)).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
			ResourceType:   rt,
			Method:         v1.OperationGet,
			HandlerFactory: operationCtrlFactory,
		})
		if err != nil {
			return err
		}
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
		err = RegisterHandler(ctx, sp, HandlerOptions{
			ParentRouter:   scopeRouter.Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
			ResourceType:   rt,
			Method:         v1.OperationPut,
			HandlerFactory: default_ctrl.NewCreateOrUpdateSubscription,
		})
		if err != nil {
			return err
		}
	}

	opStatus := fmt.Sprintf("/providers/%s/locations/{location}/operationstatuses/{operationId}", providerNamespace)
	err := RegisterHandler(ctx, sp, HandlerOptions{
		ParentRouter:   scopeRouter.Path(opStatus).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
		ResourceType:   rt,
		Method:         v1.OperationGetOperationStatuses,
		HandlerFactory: default_ctrl.NewGetOperationStatus,
	})
	if err != nil {
		return err
	}

	opResult := fmt.Sprintf("/providers/%s/locations/{location}/operationresults/{operationId}", providerNamespace)
	err = RegisterHandler(ctx, sp, HandlerOptions{
		ParentRouter:   scopeRouter.Path(opResult).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
		ResourceType:   rt,
		Method:         v1.OperationGetOperationResult,
		HandlerFactory: default_ctrl.NewGetOperationResult,
	})
	if err != nil {
		return err
	}

	return nil
}

// Responds with an HTTP 500
func internalServerError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := radlogger.GetLogger(ctx)
	logger.V(radlogger.Debug).Error(err, "unhandled error")

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
