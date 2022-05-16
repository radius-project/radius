// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Response represents a category of HTTP response (eg. OK with payload).
type Response interface {
	// Apply modifies the ResponseWriter to send the desired details back to the client.
	Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error
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

func (r *OKResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusOK), ucplog.LogHTTPStatusCode, http.StatusOK)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
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
	response := &CreatedResponse{Body: body}
	return response
}

func (r *CreatedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusCreated), ucplog.LogHTTPStatusCode, http.StatusCreated)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
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
	Scheme   string
}

func NewCreatedAsyncResponse(body interface{}, location string, scheme string) Response {
	return &CreatedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

func (r *CreatedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusCreated), ucplog.LogHTTPStatusCode, http.StatusCreated)

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
		location.Scheme = r.Scheme
	}

	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", location.String())
	w.WriteHeader(http.StatusCreated)
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
	Scheme   string
}

func NewAcceptedAsyncResponse(body interface{}, location string, scheme string) Response {
	return &AcceptedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

func (r *AcceptedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusAccepted), ucplog.LogHTTPStatusCode, http.StatusAccepted)

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
		location.Scheme = r.Scheme
	}

	logger.Info(fmt.Sprintf("Returning location: %s", location.String()))

	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", location.String())
	w.WriteHeader(http.StatusAccepted)
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

func (r *NoContentResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(204)
	return nil
}

// BadRequestResponse represents an HTTP 400 with an error message in ARM error format.
//
// This is used for any operation that fails due to bad data with a simple error message.
type BadRequestResponse struct {
	Body ErrorResponse
}

func NewBadRequestResponse(message string) Response {
	return &BadRequestResponse{
		Body: ErrorResponse{
			Error: ErrorDetails{
				Code:    Invalid,
				Message: message,
			},
		},
	}
}

func NewBadRequestARMResponse(body ErrorResponse) Response {
	return &BadRequestResponse{
		Body: body,
	}
}

func (r *BadRequestResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusBadRequest), ucplog.LogHTTPStatusCode, http.StatusBadRequest)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
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
	Body ErrorResponse
}

func NewNotFoundResponse(id string) Response {
	return &NotFoundResponse{
		Body: ErrorResponse{
			Error: ErrorDetails{
				Code:    NotFound,
				Message: fmt.Sprintf("the resource with id '%s' was not found", id),
				Target:  id,
			},
		},
	}
}

func (r *NotFoundResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusNotFound), ucplog.LogHTTPStatusCode, http.StatusNotFound)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
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
	Body ErrorResponse
}

func NewConflictResponse(message string) Response {
	return &ConflictResponse{
		Body: ErrorResponse{
			Error: ErrorDetails{
				Code:    Conflict,
				Message: message,
			},
		},
	}
}

func (r *ConflictResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusConflict), ucplog.LogHTTPStatusCode, http.StatusConflict)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

type InternalServerErrorResponse struct {
	Body ErrorResponse
}

func NewInternalServerErrorARMResponse(body ErrorResponse) Response {
	return &InternalServerErrorResponse{
		Body: body,
	}
}

func (r *InternalServerErrorResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusInternalServerError), ucplog.LogHTTPStatusCode, http.StatusInternalServerError)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

func InternalServerError(err error) Response {
	body := ErrorResponse{
		Error: ErrorDetails{
			Message: err.Error(),
		},
	}
	return NewInternalServerErrorARMResponse(body)
}
