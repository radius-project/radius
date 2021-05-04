// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/Azure/radius/pkg/curp/armerrors"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/go-playground/validator/v10"
)

// Response represents a category of HTTP response (eg. OK with payload).
type Response interface {
	// Apply modifies the ResponseWriter to send the desired details back to the client.
	Apply(w http.ResponseWriter, req *http.Request) error
}

// OKResponse represents an HTTP 200 with a JSON payload.
//
// This is used when modification to an existing resource is processed synchronously.
type OKResponse struct {
	Body interface{}
}

func NewOKResponse(body interface{}) Response {
	return &OKResponse{Body: body}
}

func (r *OKResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// CreatedResponse represents an HTTP 201 with a JSON payload.
//
// This is used when a request to create a new resource is processed synchronously.
type CreatedResponse struct {
	Body interface{}
}

func NewCreatedResponse(body interface{}) Response {
	return &CreatedResponse{Body: body}
}

func (r *CreatedResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(201)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// CreatedAsyncResponse represents an HTTP 201 with a JSON payload and location header.
//
// This is used when a request to create a new resource is processed asynchronously.
type CreatedAsyncResponse struct {
	Body     interface{}
	Location string
}

func NewCreatedAsyncResponse(body interface{}, location string) Response {
	return &CreatedAsyncResponse{Body: body, Location: location}
}

func (r *CreatedAsyncResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	location := url.URL{
		Host:   req.Host,
		Scheme: req.URL.Scheme,
		Path:   r.Location,
	}

	// In production this is the header we get from app service for the 'real' protocol
	protocol := req.Header.Get(textproto.CanonicalMIMEHeaderKey("X-Forwarded-Proto"))
	if protocol != "" {
		location.Scheme = protocol
	}

	if location.Scheme == "" {
		location.Scheme = "http"
	}

	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", location.String())
	w.WriteHeader(201)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// AcceptedAsyncResponse represents an HTTP 202 with a JSON payload and location header.
//
// This is used when a request to create an existing resource is processed asynchronously.
type AcceptedAsyncResponse struct {
	Body     interface{}
	Location string
}

func NewAcceptedAsyncResponse(body interface{}, location string) Response {
	return &AcceptedAsyncResponse{Body: body, Location: location}
}

func (r *AcceptedAsyncResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	location := url.URL{
		Host:   req.Host,
		Scheme: req.URL.Scheme,
		Path:   r.Location,
	}

	// In production this is the header we get from app service for the 'real' protocol
	protocol := req.Header.Get(textproto.CanonicalMIMEHeaderKey("X-Forwarded-Proto"))
	if protocol != "" {
		location.Scheme = protocol
	}

	if location.Scheme == "" {
		location.Scheme = "http"
	}

	log.Printf("Returning location: %s", location.String())

	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", location.String())
	w.WriteHeader(202)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// NoContentResponse represents an HTTP 204.
//
// This is used for delete operations.
type NoContentResponse struct {
}

func NewNoContentResponse() Response {
	return &NoContentResponse{}
}

func (r *NoContentResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(204)
	return nil
}

// BadRequestResponse represents an HTTP 400 with an error message in ARM error format.
//
// This is used for any operation that fails due to bad data with a simple error message.
type BadRequestResponse struct {
	Body armerrors.ErrorResponse
}

func NewBadRequestResponse(message string) Response {
	return &BadRequestResponse{
		Body: armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: message,
			},
		},
	}
}

func NewBadRequestARMResponse(body armerrors.ErrorResponse) Response {
	return &BadRequestResponse{
		Body: body,
	}
}

func (r *BadRequestResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(400)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// ValidationErrorResponse represents an HTTP 400 with validation errors in ARM error format.
type ValidationErrorResponse struct {
	Body armerrors.ErrorResponse
}

func NewValidationErrorResponse(errors validator.ValidationErrors) Response {
	body := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: errors.Error(),
		},
	}

	for _, fe := range errors {
		if err, ok := fe.(error); ok {
			detail := armerrors.ErrorDetails{
				Target:  fe.Field(),
				Message: err.Error(),
			}
			body.Error.Details = append(body.Error.Details, detail)
		}
	}

	return &ValidationErrorResponse{Body: body}
}

func (r *ValidationErrorResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(400)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// NotFoundResponse represents an HTTP 404 with an ARM error payload.
//
// This is used for GET operations when the response does not exist.
type NotFoundResponse struct {
	Body armerrors.ErrorResponse
}

func NewNotFoundResponse(id resources.ResourceID) Response {
	return &NotFoundResponse{
		Body: armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: fmt.Sprintf("the resource with id '%s' was not found", id.ID),
				Target:  id.ID,
			},
		},
	}
}

func (r *NotFoundResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(404)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// ConflictResponse represents an HTTP 409 with an ARM error payload.
//
// This is used for delete operations.
type ConflictResponse struct {
	Body armerrors.ErrorResponse
}

func NewConflictResponse(message string) Response {
	return &ConflictResponse{
		Body: armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: message,
			},
		},
	}
}

func (r *ConflictResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(409)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

type InternalServerErrorResponse struct {
	Body armerrors.ErrorResponse
}

func NewInternalServerErrorARMResponse(body armerrors.ErrorResponse) Response {
	return &InternalServerErrorResponse{
		Body: body,
	}
}

func (r *InternalServerErrorResponse) Apply(w http.ResponseWriter, req *http.Request) error {
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(500)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}
