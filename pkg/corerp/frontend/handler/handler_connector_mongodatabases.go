// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/frontend/controller/mongodatabases"
)

// Handler for Applications.Connector/MongoDatabases resource type

func (h *handlerConnector) listMongoDatabases(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	controller, err := mongodatabases.NewListMongoDatabases(h.db, h.jobEngine)
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

func (h *handlerConnector) getMongoDatabase(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	controller, err := mongodatabases.NewGetMongoDatabase(h.db, h.jobEngine)
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

func (h *handlerConnector) createOrUpdateMongoDatabase(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	controller, err := mongodatabases.NewCreateOrUpdateMongoDatabase(h.db, h.jobEngine)
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

func (h *handlerConnector) deleteMongoDatabase(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	controller, err := mongodatabases.NewDeleteMongoDatabase(h.db, h.jobEngine)
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
