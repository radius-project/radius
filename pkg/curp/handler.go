// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/rest"
)

type handler struct {
	rp ResourceProvider
}

func (h *handler) listApplications(w http.ResponseWriter, req *http.Request) {
	list, err := h.rp.ListApplications(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, list)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) getApplication(w http.ResponseWriter, req *http.Request) {
	item, err := h.rp.GetApplication(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, &item)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) updateApplication(w http.ResponseWriter, req *http.Request) {
	input := &rest.Application{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	output, err := h.rp.UpdateApplication(req.Context(), input)
	if err != nil {
		respond(w, err)
		return
	}

	err = writeCreatedResponse(w, output)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) deleteApplication(w http.ResponseWriter, req *http.Request) {
	err := h.rp.DeleteApplication(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	w.WriteHeader(204)
}

func (h *handler) listComponents(w http.ResponseWriter, req *http.Request) {
	list, err := h.rp.ListComponents(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, list)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) getComponent(w http.ResponseWriter, req *http.Request) {
	item, err := h.rp.GetComponent(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, &item)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) updateComponent(w http.ResponseWriter, req *http.Request) {
	input := &rest.Component{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	output, err := h.rp.UpdateComponent(req.Context(), input)
	if err != nil {
		respond(w, err)
		return
	}

	err = writeCreatedResponse(w, output)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) deleteComponent(w http.ResponseWriter, req *http.Request) {
	err := h.rp.DeleteComponent(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	w.WriteHeader(204)
}

func (h *handler) listDeployments(w http.ResponseWriter, req *http.Request) {
	list, err := h.rp.ListDeployments(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, list)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) getDeployment(w http.ResponseWriter, req *http.Request) {
	item, err := h.rp.GetDeployment(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, &item)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) updateDeployment(w http.ResponseWriter, req *http.Request) {
	input := &rest.Deployment{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	output, err := h.rp.UpdateDeployment(req.Context(), input)
	if err != nil {
		respond(w, err)
		return
	}

	err = writeCreatedResponse(w, output)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) deleteDeployment(w http.ResponseWriter, req *http.Request) {
	err := h.rp.DeleteDeployment(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	w.WriteHeader(204)
}

func (h *handler) listScopes(w http.ResponseWriter, req *http.Request) {
	list, err := h.rp.ListScopes(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, list)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) getScope(w http.ResponseWriter, req *http.Request) {
	item, err := h.rp.GetScope(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	err = writeOKResponse(w, &item)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) updateScope(w http.ResponseWriter, req *http.Request) {
	input := &rest.Scope{}
	err := readJSONResource(req, input, resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	output, err := h.rp.UpdateScope(req.Context(), input)
	if err != nil {
		respond(w, err)
		return
	}

	err = writeCreatedResponse(w, output)
	if err != nil {
		respond(w, err)
		return
	}
}

func (h *handler) deleteScope(w http.ResponseWriter, req *http.Request) {
	err := h.rp.DeleteScope(req.Context(), resourceID(req))
	if err != nil {
		respond(w, err)
		return
	}

	w.WriteHeader(204)
}

func resourceID(req *http.Request) resources.ResourceID {
	id, err := resources.Parse(req.URL.Path)
	if err != nil {
		log.Printf("URL was not a valid resource id: %v", req.URL.Path)
		// just log the error - it will be handled in the RP layer.
	}
	return id
}

func respond(w http.ResponseWriter, err error) {
	if st, ok := err.(StatusCodeError); ok {
		log.Printf("responding with %d: %v", st.StatusCode(), err)

		obj := st.ErrorResponse()
		bytes, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			log.Printf("error marshaling %T: %v", obj, err)
			w.WriteHeader(500)
			return
		}

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(st.StatusCode())
		_, err = w.Write(bytes)
		if err != nil {
			// There's no way to recover if we fail writing here, we already set the stautus
			// code and likly partially wrote to the response stream.
			log.Printf("error writing marshaled %T bytes to output: %s", obj, err)
		}

		return
	}

	log.Printf("responding with 500: %v", err)
	w.WriteHeader(500)
}

func readJSONResource(req *http.Request, obj rest.Resource, id resources.ResourceID) error {
	if req.Body == nil {
		return BadRequestError{"request does not have a body"}
	}

	defer req.Body.Close()
	err := json.NewDecoder(req.Body).Decode(obj)
	if err != nil {
		return fmt.Errorf("error reading %T: %w", obj, err)
	}

	obj.SetID(id)

	return nil
}

func writeOKResponse(w http.ResponseWriter, obj interface{}) error {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", obj, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", obj, err)
	}

	return nil
}

func writeCreatedResponse(w http.ResponseWriter, obj rest.Resource) error {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", obj, err)
	}

	id, err := obj.GetID()
	if err != nil {
		return fmt.Errorf("%T does not have a value resource id: %w", obj, err)
	}

	w.Header().Add("Location", id.ID)
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(201)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", obj, err)
	}

	return nil
}
