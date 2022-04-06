// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

// A brief note on error handling... The handler is responsible for all of the direct actions
// with HTTP request/reponse.
//
// The RP returns the rest.Response type for "known" or "expected" error conditions:
// - validation error
// - missing data
//
// The RP returns an error for "unexpected" error conditions:
// - DB failure
// - I/O failure
//
// This code will assume that any Golang error returned from the RP represents a reliability error
// within the RP or a bug.

type handler struct {
	db        db.RadrpDB
	jobEngine deployment.DeploymentProcessor

	pathBase string
}

func (h *handler) GetOperations(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	action := &provider_ctrl.GetOperations{
		BaseController: ctrl.BaseController{
			DBProvider: h.db,
			JobEngine:  h.jobEngine,
		},
	}

	response, err := action.Run(ctx, req)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}
}

func (h *handler) CreateOrUpdateSubscription(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	action := &provider_ctrl.CreateOrUpdateSubscription{
		BaseController: ctrl.BaseController{
			DBProvider: h.db,
			JobEngine:  h.jobEngine,
		},
	}

	response, err := action.Run(ctx, req)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}
}

func (h *handler) ListEnvironments(w http.ResponseWriter, req *http.Request) {
	// TODO: Implement environment resource type list operations
	ctx := req.Context()
	log := radlogger.GetLogger(ctx)
	rpcCtx := servicecontext.ARMRequestContextFromContext(ctx)
	log.Info(fmt.Sprintf("api-version: %s", rpcCtx.APIVersion))

	internalServerError(ctx, w, req, errors.New("Not implemented"))
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
