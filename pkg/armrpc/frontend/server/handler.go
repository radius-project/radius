// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

type ControllerFunc func(store.StorageClient, manager.StatusManager) (ctrl.Controller, error)

type handlerParam struct {
	parent       *mux.Router
	resourcetype string
	method       string
	routeName    string
	fn           ControllerFunc
}

func RegisterHandler(ctx context.Context, sp dataprovider.DataStorageProvider, parent *mux.Router, resourcetype string, method string, operationMethod string, createControllerFn ControllerFunc) error {
	sc, err := sp.GetStorageClient(ctx, resourcetype)
	if err != nil {
		return err
	}

	ctrl, err := createControllerFn(sc, nil)
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

	ot := v1.OperationType{Type: resourcetype, Method: operationMethod}
	parent.Methods(method).HandlerFunc(fn).Name(ot.String())
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
