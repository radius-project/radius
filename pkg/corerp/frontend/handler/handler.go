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
)

type ControllerFunc func(store.StorageClient, deployment.DeploymentProcessor) (controller.ControllerInterface, error)

type handlerParam struct {
	parent       *mux.Router
	resourcetype string
	method       string
	routeName    string
	fn           ControllerFunc
}

func registerHandler(ctx context.Context, sp dataprovider.DataStorageProvider, parent *mux.Router, resourcetype string, method string, routeName string, CreateController ControllerFunc) error {
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
	parent.Methods(method).HandlerFunc(fn).Name(routeName)
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
