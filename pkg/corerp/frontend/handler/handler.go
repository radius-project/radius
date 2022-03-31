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
	"strings"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/frontend/controllers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
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
	providerCtrl *controllers.ProviderController
	appCoreCtrl  *controllers.AppCoreController

	validatorFactory ValidatorFactory
	pathPrefix       string
}

func (h *handler) GetOperations(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.appCoreCtrl.GetOperations(ctx, h.resourceID(req))
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
	response, err := h.providerCtrl.CreateOrUpdateSubscription(ctx, h.resourceID(req))
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
	ctx := req.Context()

	// TODO: Implement environment resource type list operations
	internalServerError(ctx, w, req, errors.New("Not implemented"))
}

func (h *handler) resourceID(req *http.Request) azresources.ResourceID {
	logger := radlogger.GetLogger(req.Context())
	path := req.URL.Path
	pathFixed := strings.TrimPrefix(path, h.pathPrefix)
	id, err := azresources.Parse(pathFixed)
	if err != nil {
		logger.Info("URL was not a valid resource id: %v", req.URL.Path)
		// just log the error - it will be handled in the RP layer.
	}
	return id
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
