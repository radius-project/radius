// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
)

// A brief note on error handling: The handler is responsible for all of the direct actions
// with HTTP request/reponse.
//
// The RP returns the rest.Response type for "known" or "expected" error conditions:
// - validation error
// - missing data
//
// The RP returns an error for "unexpected" error conditions:
// - DB failure
// - I/O failure
// This code will assume that any error returned from the RP represents a reliability error
// within the RP or a bug.

type handlerConnector struct {
	db        db.RadrpDB
	jobEngine deployment.DeploymentProcessor

	pathBase string
}

func (h *handlerConnector) getOperations(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	controller, err := provider.NewGetConnectorOperations(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := controller.Run(ctx, req)
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

func (h *handlerConnector) createOrUpdateSubscription(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	controller, err := provider.NewCreateOrUpdateSubscriptionConnector(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := controller.Run(ctx, req)
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
