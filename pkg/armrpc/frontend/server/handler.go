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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	default_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/defaultcontroller"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

const (
	APIVersionParam = "api-version"
)

type ControllerFunc func(ctrl.Options) (ctrl.Controller, error)

type HandlerOptions struct {
	ParentRouter   *mux.Router
	ResourceType   string
	Method         v1.OperationMethod
	HandlerFactory ControllerFunc
}

func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts ctrl.Options) error {
	storageClient, err := ctrlOpts.DataProvider.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}
	ctrlOpts.StorageClient = storageClient
	ctrlOpts.ResourceType = opts.ResourceType

	ctrl, err := opts.HandlerFactory(ctrlOpts)
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
	rootRouter *mux.Router,
	pathBase string,
	isAzureProvider bool,
	providerNamespace string,
	operationCtrlFactory ControllerFunc,
	ctrlOpts ctrl.Options) error {
	providerNamespace = strings.ToLower(providerNamespace)
	rt := providerNamespace + "/provider"

	if isAzureProvider {
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		err := RegisterHandler(ctx, HandlerOptions{
			ParentRouter:   rootRouter.Path(fmt.Sprintf("/providers/%s/operations", providerNamespace)).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
			ResourceType:   rt,
			Method:         v1.OperationGet,
			HandlerFactory: operationCtrlFactory,
		}, ctrlOpts)
		if err != nil {
			return err
		}
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
		err = RegisterHandler(ctx, HandlerOptions{
			ParentRouter:   rootRouter.Path(pathBase).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
			ResourceType:   rt,
			Method:         v1.OperationPut,
			HandlerFactory: default_ctrl.NewCreateOrUpdateSubscription,
		}, ctrlOpts)
		if err != nil {
			return err
		}
	}

	statusRT := providerNamespace + "/operationstatuses"
	opStatus := fmt.Sprintf("%s/providers/%s/locations/{location}/operationstatuses/{operationId}", pathBase, providerNamespace)
	err := RegisterHandler(ctx, HandlerOptions{
		ParentRouter:   rootRouter.Path(opStatus).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
		ResourceType:   statusRT,
		Method:         v1.OperationGetOperationStatuses,
		HandlerFactory: default_ctrl.NewGetOperationStatus,
	}, ctrlOpts)
	if err != nil {
		return err
	}

	opResult := fmt.Sprintf("%s/providers/%s/locations/{location}/operationresults/{operationId}", pathBase, providerNamespace)
	err = RegisterHandler(ctx, HandlerOptions{
		ParentRouter:   rootRouter.Path(opResult).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
		ResourceType:   statusRT,
		Method:         v1.OperationGetOperationResult,
		HandlerFactory: default_ctrl.NewGetOperationResult,
	}, ctrlOpts)
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
