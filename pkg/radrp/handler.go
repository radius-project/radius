// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
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
	response, err := h.rp.ListApplications(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) getApplication(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.GetApplication(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) updateApplication(w http.ResponseWriter, req *http.Request) {
	input := &rest.Application{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(w, err)
		return
	}

	response, err := h.rp.UpdateApplication(req.Context(), input)
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) deleteApplication(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.DeleteApplication(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) listComponents(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.ListComponents(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) getComponent(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.GetComponent(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) updateComponent(w http.ResponseWriter, req *http.Request) {
	// Treat invalid input as a bad request
	input := &rest.Component{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(w, err)
		return
	}

	response, err := h.rp.UpdateComponent(req.Context(), input)
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
	fmt.Println("@@@@ Done updatecomponent")
}

func (h *handler) deleteComponent(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.DeleteComponent(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) listDeployments(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.ListDeployments(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) getDeployment(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.GetDeployment(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) updateDeployment(w http.ResponseWriter, req *http.Request) {
	input := &rest.Deployment{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(w, err)
		return
	}

	response, err := h.rp.UpdateDeployment(req.Context(), input)
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) deleteDeployment(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.DeleteDeployment(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) listScopes(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.ListScopes(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) getScope(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.GetScope(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) updateScope(w http.ResponseWriter, req *http.Request) {
	input := &rest.Scope{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		badRequest(w, err)
		return
	}

	response, err := h.rp.UpdateScope(req.Context(), input)
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) deleteScope(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.DeleteScope(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func (h *handler) getDeploymentOperation(w http.ResponseWriter, req *http.Request) {
	response, err := h.rp.GetDeploymentOperationByID(req.Context(), resourceID(req))
	if err != nil {
		internalServerError(w, err)
		return
	}

	err = response.Apply(w, req)
	if err != nil {
		internalServerError(w, err)
		return
	}
}

func resourceID(req *http.Request) resources.ResourceID {
	id, err := resources.Parse(req.URL.Path)
	if err != nil {
		log.Printf("URL was not a valid resource id: %v", req.URL.Path)
		// just log the error - it will be handled in the RP layer.
	}
	return id
}

func badRequest(w http.ResponseWriter, err error) {
	// Try to use the ARM format to send back the error info
	body := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: err.Error(),
		},
	}

	// If we fail to serialize the error, log it and just reply with a 'plain' 500
	bytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		log.Printf("error marshaling %T: %v", body, err)
		w.WriteHeader(500)
		return
	}

	if err != nil {
		log.Printf("error marshaling %T: %v", body, err)
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(400)
	_, err = w.Write(bytes)
	if err != nil {
		// There's no way to recover if we fail writing here, we already set the stautus
		// code and likly partially wrote to the response stream.
		log.Printf("error writing marshaled %T bytes to output: %s", body, err)
	}
}

// Responds with an HTTP 500
func internalServerError(w http.ResponseWriter, err error) {
	// Try to use the ARM format to send back the error info
	body := &armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: err.Error(),
		},
	}

	// If we fail to serialize the error, log it and just reply with a 'plain' 500
	bytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		log.Printf("error marshaling %T: %v", body, err)
		w.WriteHeader(500)
		return
	}

	if err != nil {
		log.Printf("error marshaling %T: %v", body, err)
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(500)
	_, err = w.Write(bytes)
	if err != nil {
		// There's no way to recover if we fail writing here, we already set the stautus
		// code and likly partially wrote to the response stream.
		log.Printf("error writing marshaled %T bytes to output: %s", body, err)
	}
}

func readJSONResource(req *http.Request, obj rest.Resource, id resources.ResourceID) error {
	defer req.Body.Close()
	err := json.NewDecoder(req.Body).Decode(obj)
	if err != nil {
		return fmt.Errorf("error reading %T: %w", obj, err)
	}

	// Set Resource properties on the resource based on the URL
	obj.SetID(id)

	return nil
}
