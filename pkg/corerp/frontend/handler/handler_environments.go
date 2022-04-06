// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"net/http"

	environments_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
)

// This includes the handlers for environment resource type.

func (h *handler) listEnvironments(w http.ResponseWriter, req *http.Request) {
	// TODO: Implement environment resource type list operations
	ctx := req.Context()
	op, err := environments_ctrl.NewListEnvironments(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := op.Run(ctx, req)
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

func (h *handler) getEnvironment(w http.ResponseWriter, req *http.Request) {
	// TODO: Implement environment resource type list operations
	ctx := req.Context()
	op, err := environments_ctrl.NewGetEnvironment(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := op.Run(ctx, req)
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

func (h *handler) createOrUpdateEnvironment(w http.ResponseWriter, req *http.Request) {
	// TODO: Implement environment resource type list operations
	ctx := req.Context()
	op, err := environments_ctrl.NewCreateOrUpdateEnvironment(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := op.Run(ctx, req)
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

func (h *handler) deleteEnvironment(w http.ResponseWriter, req *http.Request) {
	// TODO: Implement environment resource type list operations
	ctx := req.Context()
	op, err := environments_ctrl.NewDeleteEnvironment(h.db, h.jobEngine)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	response, err := op.Run(ctx, req)
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
