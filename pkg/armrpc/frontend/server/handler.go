// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

const (
	APIVersionParam = "api-version"
)

type ControllerFunc func(ctrl.Options) (ctrl.Controller, error)

type HandlerOptions struct {
	ParentRouter   *mux.Router
	ResourceType   string
	Path           string
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
		addRequestAttributes(hctx, req)

		response, err := ctrl.Run(hctx, w, req)
		if err != nil {
			HandleError(hctx, w, req, err)
			return
		}

		err = response.Apply(hctx, w, req)
		if err != nil {
			HandleError(hctx, w, req, err)
			return
		}
	}

	ot := v1.OperationType{Type: opts.ResourceType, Method: opts.Method}
	opts.ParentRouter.Methods(opts.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())

	return nil
}

func addRequestAttributes(ctx context.Context, req *http.Request) {
	labeler, ok := otelhttp.LabelerFromContext(ctx)
	if !ok {
		return
	}

	armContext := v1.ARMRequestContextFromContext(ctx)
	resourceID := armContext.ResourceID

	if resourceID.IsResource() || resourceID.IsResourceCollection() {
		labeler.Add(attribute.String("resource_type", strings.ToLower(resourceID.Type())))
		labeler.Add(attribute.String("ucp_plane", strings.ToLower(resourceID.PlaneNamespace())))
	}
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
			HandlerFactory: defaultoperation.NewCreateOrUpdateSubscription,
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
		HandlerFactory: defaultoperation.NewGetOperationStatus,
	}, ctrlOpts)
	if err != nil {
		return err
	}

	opResult := fmt.Sprintf("%s/providers/%s/locations/{location}/operationresults/{operationId}", pathBase, providerNamespace)
	err = RegisterHandler(ctx, HandlerOptions{
		ParentRouter:   rootRouter.Path(opResult).Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter(),
		ResourceType:   statusRT,
		Method:         v1.OperationGetOperationResult,
		HandlerFactory: defaultoperation.NewGetOperationResult,
	}, ctrlOpts)
	if err != nil {
		return err
	}

	return nil
}

// Responds with an HTTP 500
func HandleError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Error(err, "unhandled error")

	var response rest.Response
	// Try to use the ARM format to send back the error info
	// if the error is due to api conversion failure return bad resquest
	switch v := err.(type) {
	case *v1.ErrModelConversion:
		response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
				Message: err.Error(),
			},
		})
	case *v1.ErrClientRP:
		response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v.Code,
				Message: v.Message,
			},
		})
	default:
		if errors.Is(err, v1.ErrInvalidModelConversion) {
			response = rest.NewBadRequestARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
					Message: err.Error(),
				},
			})
		} else {
			response = rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: err.Error(),
				},
			})
		}
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		body := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: err.Error(),
			},
		}
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}
