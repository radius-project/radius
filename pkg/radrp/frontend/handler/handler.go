// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
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

type Handler struct {
	Rp               resourceprovider.ResourceProvider
	ValidatorFactory ValidatorFactory
	PathPrefix       string
}

func (h *Handler) ListApplications(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.ListApplications(ctx, h.resourceID(req))
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

func (h *Handler) GetApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.GetApplication(ctx, h.resourceID(req))
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

func (h *Handler) UpdateApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	body, err := readJSONBody(req)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	id := h.resourceID(req)
	validator, err := h.findValidator(id)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	validationErrs := validator.ValidateJSON(body)
	if len(validationErrs) > 0 {
		validationError(ctx, w, req, validationErrs)
		return
	}

	response, err := h.Rp.UpdateApplication(ctx, id, body)
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

func (h *Handler) DeleteApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.DeleteApplication(ctx, h.resourceID(req))
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

func (h *Handler) ListAllV3ResourcesByApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	response, err := h.Rp.ListAllV3ResourcesByApplication(ctx, h.resourceID(req))
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

func (h *Handler) ListResources(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	response, err := h.Rp.ListResources(ctx, h.resourceID(req))
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

func (h *Handler) GetResource(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.GetResource(ctx, h.resourceID(req))
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

func (h *Handler) UpdateResource(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	body, err := readJSONBody(req)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	id := h.resourceID(req)
	validator, err := h.findValidator(id)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	validationErrs := validator.ValidateJSON(body)
	if len(validationErrs) > 0 {
		validationError(ctx, w, req, validationErrs)
		return
	}

	response, err := h.Rp.UpdateResource(ctx, id, body)
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

func (h *Handler) DeleteResource(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.DeleteResource(ctx, h.resourceID(req))
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

func (h *Handler) GetOperation(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.Rp.GetOperation(ctx, h.resourceID(req))
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

func (h *Handler) ListSecrets(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	body, err := readJSONBody(req)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	input := resourceprovider.ListSecretsInput{}
	err = json.Unmarshal(body, &input)
	if err != nil {
		badRequest(ctx, w, req, err)
		return
	}

	response, err := h.Rp.ListSecrets(ctx, input)
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

func (h *Handler) GetSwaggerDoc(w http.ResponseWriter, req *http.Request) {
	// Required for the K8s scenario, we are required to respond to a request
	// to /apis/api.radius.dev/v1alpha3 with a 200 OK response.
	ctx := req.Context()
	response, err := h.Rp.GetSwaggerDoc(ctx)
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

func (h *Handler) findValidator(id azresources.ResourceID) (schema.Validator, error) {
	resourceType := id.Types[len(id.Types)-1].Type
	return h.ValidatorFactory(resourceType)
}

func (h *Handler) resourceID(req *http.Request) azresources.ResourceID {
	logger := radlogger.GetLogger(req.Context())
	path := req.URL.Path
	pathFixed := strings.TrimPrefix(path, h.PathPrefix)
	id, err := azresources.Parse(pathFixed)
	if err != nil {
		logger.Info("URL was not a valid resource id: %v", req.URL.Path)
		// just log the error - it will be handled in the RP layer.
	}
	return id
}

func validationError(ctx context.Context, w http.ResponseWriter, req *http.Request, validationErrs []schema.ValidationError) {
	logger := radlogger.GetLogger(ctx)

	body := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: "Validation error",
			Details: make([]armerrors.ErrorDetails, len(validationErrs)),
		},
	}

	for i, err := range validationErrs {
		if err.JSONError != nil {
			// The given document isn't even JSON.
			body.Error.Details[i].Message = fmt.Sprintf("%s: %v", err.Message, err.JSONError)
		} else {
			body.Error.Details[i].Message = fmt.Sprintf("%s: %s", err.Position, err.Message)
		}
	}

	response := rest.NewBadRequestARMResponse(body)
	err := response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}

func badRequest(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := radlogger.GetLogger(ctx)
	// Try to use the ARM format to send back the error info
	body := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Code:    armerrors.Invalid,
			Message: err.Error(),
		},
	}

	response := rest.NewBadRequestARMResponse(body)
	err = response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
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

func readJSONBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	return data, nil
}
