// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

type Handler struct {
	db  store.StorageClient
	ucp ucphandler.UCPHandler
}

func (h *Handler) GetDiscoveryDoc(w http.ResponseWriter, req *http.Request) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3 with a 200 OK response and the following
	// format.
	//
	// This tells the API Server we don't serve any CRDs (empty list).
	ctx := req.Context()

	// We avoid using the rest package here so we can avoid logging every request.
	// This endpoint is called ..... A ... LOT.
	b, err := json.Marshal(map[string]interface{}{
		"kind":         "APIResourceList",
		"apiVersion":   "v1alpha3",
		"groupVersion": "api.ucp.dev/v1alpha3",
		"resources":    []interface{}{},
	})
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	w.WriteHeader(200)
	w.Header().Add("Content-Type", "application/json")

	_, err = w.Write(b)
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}
}

func (h *Handler) GetOpenAPIv2Doc(w http.ResponseWriter, req *http.Request) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.ucp.dev/v1alpha3/openapi/v2 with a 200 OK response and a swagger (openapi v2)
	// doc.
	//
	// We don't need this for any functionality, but it will make the API server happy.
	ctx := req.Context()

	// We avoid using the rest package here so we can avoid logging every request.
	// This endpoint is called ..... A ... LOT.
	b, err := json.Marshal(map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":   "Radius APIService",
			"version": "v1alpha3",
		},
		"paths": map[string]interface{}{},
	})
	if err != nil {
		internalServerError(ctx, w, req, err)
		return
	}

	w.WriteHeader(200)
	w.Header().Add("Content-Type", "application/json")

	_, err = w.Write(b)
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

	logger.Info("starting proxy request", "url", r.URL.String(), "method", r.Method)
	for key, value := range r.Header {
		logger.V(4).Info("incoming request header", "key", key, "value", value)
	}

	r.URL.Path = h.getRelativePath(r.URL.Path)

	// Make a copy of the incoming URL and trim the base path
	newURL := *r.URL
	newURL.Path = h.getRelativePath(r.URL.Path)
	response, err := h.ucp.Planes.ProxyRequest(ctx, h.db, w, r, &newURL)
	if err != nil {
		internalServerError(ctx, w, r, err)
		return
	}

	if response != nil {
		err = response.Apply(ctx, w, r)
		if err != nil {
			internalServerError(ctx, w, r, err)
			return
		}
	}
}
func (h *Handler) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	path := h.getRelativePath(r.URL.Path)
	restResponse := rest.NewNotFoundResponse(path)
	err := restResponse.Apply(r.Context(), w, r)
	if err != nil {
		internalServerError(r.Context(), w, r, err)
		return
	}
}

func (h *Handler) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	path := h.getRelativePath(r.URL.Path)
	target := ""
	if rID, err := resources.Parse(path); err != nil {
		target = rID.Type() + "/" + rID.Name()
	}
	restResponse := rest.NewMethodNotAllowedResponse(target, fmt.Sprintf("The request method '%s' is invalid.", r.Method))
	if err := restResponse.Apply(r.Context(), w, r); err != nil {
		internalServerError(r.Context(), w, r, err)
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
	response, err := h.ucp.ResourceGroups.DeleteByID(ctx, h.db, h.getRelativePath(r.URL.Path), r)
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
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}
