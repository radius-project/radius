// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
)

// A brief not on error handling... The handler is responsible for all of the direct actions
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
// This code will assume that any error returned from the RP represents a reliability error
// within the RP or a bug.

type handler struct {
	rp ResourceProvider
}

func (h *handler) listApplications(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.ListApplications(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) getApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.GetApplication(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) updateApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	input := &rest.Application{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(ctx, w, err)
		return
	}

	response, err := h.rp.UpdateApplication(ctx, input)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) deleteApplication(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.DeleteApplication(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) listComponents(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.ListComponents(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) getComponent(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.GetComponent(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) updateComponent(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	// Treat invalid input as a bad request
	input := &rest.Component{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(ctx, w, err)
		return
	}

	response, err := h.rp.UpdateComponent(ctx, input)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) deleteComponent(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.DeleteComponent(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) listDeployments(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.ListDeployments(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) getDeployment(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.GetDeployment(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) updateDeployment(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	input := &rest.Deployment{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(ctx, w, err)
		return
	}

	response, err := h.rp.UpdateDeployment(ctx, input)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) deleteDeployment(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.DeleteDeployment(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) listScopes(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.ListScopes(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) getScope(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.GetScope(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) updateScope(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	input := &rest.Scope{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(ctx, w, err)
		return
	}

	response, err := h.rp.UpdateScope(ctx, input)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) deleteScope(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.DeleteScope(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func (h *handler) getDeploymentOperation(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	response, err := h.rp.GetDeploymentOperationByID(ctx, resourceID(req))
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}

	err = response.Apply(ctx, w, req)
	if err != nil {
		internalServerError(ctx, w, err)
		return
	}
}

func resourceID(req *http.Request) resources.ResourceID {
	logger := radlogger.GetLogger(req.Context())
	id, err := resources.Parse(req.URL.Path)
	if err != nil {
		logger.Info("URL was not a valid resource id: %v", req.URL.Path)
		// just log the error - it will be handled in the RP layer.
	}
	return id
}

func badRequest(ctx context.Context, w http.ResponseWriter, err error) {
	logger := radlogger.GetLogger(ctx)
	validationErr, ok := err.(*validationError)
	var body *armerrors.ErrorResponse
	if !ok {
		// Try to use the ARM format to send back the error info
		body = &armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: err.Error(),
			},
		}
	} else {
		body = &armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.Invalid,
				Message: "Validation error",
				Details: make([]armerrors.ErrorDetails, len(validationErr.details)),
			},
		}
		for i, err := range validationErr.details {
			body.Error.Details[i].Message = fmt.Sprintf("%s: %s", err.Position, err.Message)
		}
	}

	// If we fail to serialize the error, log it and just reply with a 'plain' 500
	bytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		logger.Error(err, fmt.Sprintf("error marshaling %T", body))
		w.WriteHeader(500)
		return
	}

	if err != nil {
		logger.Error(err, fmt.Sprintf("error marshaling %T", body))
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(400)
	_, err = w.Write(bytes)
	if err != nil {
		// There's no way to recover if we fail writing here, we already set the stautus
		// code and likly partially wrote to the response stream.
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}

// Responds with an HTTP 500
func internalServerError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := radlogger.GetLogger(ctx)
	// Try to use the ARM format to send back the error info
	body := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: err.Error(),
		},
	}

	// If we fail to serialize the error, log it and just reply with a 'plain' 500
	bytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		logger.Error(err, fmt.Sprintf("error marshaling %T", body))
		w.WriteHeader(500)
		return
	}

	if err != nil {
		logger.Error(err, fmt.Sprintf("error marshaling %T", body))
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(500)
	_, err = w.Write(bytes)
	if err != nil {
		// There's no way to recover if we fail writing here, we already set the stautus
		// code and likly partially wrote to the response stream.
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}

func readJSONResource(req *http.Request, obj rest.Resource, id resources.ResourceID) error {
	defer req.Body.Close()
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	validator, err := schema.ValidatorFor(obj)
	if err != nil {
		return fmt.Errorf("cannot find validator for %T: %w", obj, err)
	}
	if errs := validator.ValidateJSON(data); len(errs) != 0 {
		return &validationError{
			details: errs,
		}
	}
	err = json.Unmarshal(data, obj)
	if err != nil {
		return fmt.Errorf("error reading %T: %w", obj, err)
	}

	// Set Resource properties on the resource based on the URL
	obj.SetID(id)

	return nil
}

type validationError struct {
	details []schema.ValidationError
}

func (v *validationError) Error() string {
	var message strings.Builder
	fmt.Fprintln(&message, "failed validation(s):")
	for _, err := range v.details {
		if err.JSONError != nil {
			// The given document isn't even JSON.
			fmt.Fprintf(&message, "- %s: %v\n", err.Message, err.JSONError)
		} else {
			fmt.Fprintf(&message, "- %s: %s\n", err.Position, err.Message)
		}
	}
	return message.String()
}
