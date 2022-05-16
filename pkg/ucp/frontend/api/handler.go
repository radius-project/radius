// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package api

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

type Handler struct {
	db  store.StorageClient
	ucp ucphandler.UCPHandler
}

func (h *Handler) GetSwaggerDoc(w http.ResponseWriter, req *http.Request) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3 with a 200 OK response.
	ctx := req.Context()
	response := rest.NewOKResponse([]byte{})

	err := response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}
}

func (h *Handler) getRelativePath(path string) string {
	trimmedPath := strings.TrimPrefix(path, h.ucp.Options.BasePath)
	return trimmedPath
}

func (h *Handler) CreateOrUpdatePlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := readJSONBody(r)
	if err != nil {
		badRequest(ctx, w, r, err)
		return
	}
	//TODO: Validate against schema

	response, err := h.ucp.Planes.CreateOrUpdate(ctx, h.db, body, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}
func (h *Handler) ListPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.Planes.List(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}
func (h *Handler) GetPlaneByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.Planes.GetByID(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}
func (h *Handler) DeletePlaneByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.Planes.DeleteByID(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}

func (h *Handler) ProxyPlaneRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := ucplog.GetLogger(ctx)

	logger.V(3).Info("starting proxy request", "url", r.URL.String(), "method", r.Method)
	for key, value := range r.Header {
		logger.V(4).Info("incoming request header", "key", key, "value", value)
	}

	response, err := h.ucp.Planes.ProxyRequest(ctx, h.db, w, r, h.getRelativePath(r.URL.Path))
	if err != nil {
		err := response.Apply(ctx, w, r)
		if err != nil {
			internalServerError(ctx, w, r, err)
			return
		}
	}
}
func (h *Handler) DefaultHandler(w http.ResponseWriter, r *http.Request) {
	restResponse := rest.NewNotFoundResponse(r.URL.Path)
	err := restResponse.Apply(r.Context(), w, r)
	if err != nil {
		internalServerError(r.Context(), w, r, err)
		return
	}
}

func (h *Handler) CreateResourceGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := readJSONBody(r)
	if err != nil {
		badRequest(ctx, w, r, err)
		return
	}
	response, err := h.ucp.ResourceGroups.Create(ctx, h.db, body, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}

func (h *Handler) ListResourceGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.ResourceGroups.List(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}
func (h *Handler) GetResourceGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.ResourceGroups.GetByID(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}
func (h *Handler) DeleteResourceGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response, err := h.ucp.ResourceGroups.DeleteByID(ctx, h.db, h.getRelativePath(r.URL.Path))
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
	err = response.Apply(ctx, w, r)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}
}

func badRequest(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := ucplog.GetLogger(req.Context())
	// Try to use the ARM format to send back the error info
	response := rest.NewBadRequestResponse(err.Error())
	err = response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, "error while writing marshalled response")
	}
}

// Responds with an HTTP 500
func internalServerError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := ucplog.GetLogger(ctx)
	logger.Error(err, "unhandled error")
	// Try to use the ARM format to send back the error info
	body := rest.ErrorResponse{
		Error: rest.ErrorDetails{
			Message: err.Error(),
		},
	}
	response := rest.NewInternalServerErrorARMResponse(body)
	err = response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likely partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, "error while writing marshalled response")
	}
}
func readJSONBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}
