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
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/logging"

	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	LinkedResourceUpdateErrorFormat = "Attempted to deploy existing resource '%s' which has a different application and/or environment. Options to resolve the conflict are: change the name of the '%s' resource in %s to create a new resource, or use '%s' application and '%s' environment to update the existing resource '%s'."
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
	Body    any
	Headers map[string]string
}

// NewOKResponse creates an OKResponse that will write a 200 OK with the provided body as JSON.
// Set the body to nil to write an empty 200 OK.
func NewOKResponse(body any) Response {
	return &OKResponse{Body: body}
}

// NewOKResponseWithHeaders creates an OKResponse that will write a 200 OK with the provided body as JSON.
// Set the body to nil to write an empty 200 OK.
func NewOKResponseWithHeaders(body any, headers map[string]string) Response {
	return &OKResponse{
		Body:    body,
		Headers: headers,
	}
}

func (r *OKResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusOK), logging.LogHTTPStatusCode, http.StatusOK)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	for key, element := range r.Headers {
		w.Header().Add(key, element)
	}

	if r.Body == nil {
		w.WriteHeader(http.StatusOK)
		return nil
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
	Body any
}

func NewCreatedResponse(body any) Response {
	response := &CreatedResponse{Body: body}
	return response
}

func (r *CreatedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusCreated), logging.LogHTTPStatusCode, http.StatusCreated)

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
	Body     any
	Location string
	Scheme   string
}

func NewCreatedAsyncResponse(body any, location string, scheme string) Response {
	return &CreatedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

func (r *CreatedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusCreated), logging.LogHTTPStatusCode, http.StatusCreated)

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
	Body     any
	Location string
	Scheme   string
}

// NewAcceptedAsyncResponse creates an AcceptedAsyncResponse
func NewAcceptedAsyncResponse(body any, location string, scheme string) Response {
	return &AcceptedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

func (r *AcceptedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusAccepted), logging.LogHTTPStatusCode, http.StatusAccepted)

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

// AsyncOperationResponse represents the response for an async operation request.
type AsyncOperationResponse struct {
	Body        any
	Location    string
	Code        int
	ResourceID  resources.ID
	OperationID uuid.UUID
	APIVersion  string
	RootScope   string // Everything before providers namespace for constructing an Async operation header. Used for AWS planes
	PathBase    string // Base Path. Used for AWS planes
}

// NewAsyncOperationResponse creates an AsyncOperationResponse
func NewAsyncOperationResponse(body any, location string, code int, resourceID resources.ID, operationID uuid.UUID, apiVersion string, rootScope string, pathBase string) Response {
	return &AsyncOperationResponse{
		Body:        body,
		Location:    location,
		Code:        code,
		ResourceID:  resourceID,
		OperationID: operationID,
		APIVersion:  apiVersion,
		RootScope:   rootScope,
		PathBase:    pathBase,
	}
}

func (r *AsyncOperationResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	// Write Body
	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	locationHeader, err := r.getAsyncLocationPath(req, "operationResults")
	if err != nil {
		return err
	}
	azureAsyncOpHeader, err := r.getAsyncLocationPath(req, "operationStatuses")
	if err != nil {
		return err
	}

	// Write Headers
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Original referer header: %s", req.Header.Get(v1.RefererHeader)))
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", locationHeader)
	logger.Info(fmt.Sprintf("Configured Location header: %s", locationHeader))
	w.Header().Add("Azure-AsyncOperation", azureAsyncOpHeader)
	logger.Info(fmt.Sprintf("Configured AsyncOperation header: %s", azureAsyncOpHeader))
	w.Header().Add("Retry-After", v1.DefaultRetryAfter)
	w.Header().Add("Referer", req.Header.Get(v1.RefererHeader))

	w.WriteHeader(r.Code)

	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// getAsyncLocationPath returns the async operation location path for the given resource type.
func (r *AsyncOperationResponse) getAsyncLocationPath(req *http.Request, resourceType string) (string, error) {
	rootScope := r.RootScope
	if rootScope == "" {
		rootScope = r.ResourceID.PlaneScope()
	}

	referer, err := url.Parse(req.Header.Get(v1.RefererHeader))
	logger := logr.FromContextOrDiscard(req.Context())
	logger.Info("og referer header from request: " + req.Header.Get(v1.RefererHeader))
	logger.Info("Referer host: " + referer.Host)
	if err != nil {
		return "", err
	}
	baseIndex := 0
	if referer.Path != "" {
		baseIndex = getBaseIndex(referer.Path)
	}
	base := referer.Path[:baseIndex]
	logger.Info("Referer base path: " + base)

	dest := url.URL{
		Host:   referer.Host,
		Scheme: referer.Scheme,
		Path:   fmt.Sprintf("%s%s/providers/%s/locations/%s/%s/%s", base, rootScope, r.ResourceID.ProviderNamespace(), r.Location, resourceType, r.OperationID.String()),
	}
	fmt.Println(dest.String())

	query := url.Values{}
	if r.APIVersion != "" {
		query.Add("api-version", r.APIVersion)
	}

	dest.RawQuery = query.Encode()

	// In production this is the header we get from app service for the 'real' protocol
	protocol := req.Header.Get("X-Forwarded-Proto")
	if protocol != "" {
		dest.Scheme = protocol
	}

	if dest.Scheme == "" && req.TLS != nil {
		dest.Scheme = "https"
	} else if dest.Scheme == "" {
		dest.Scheme = "http"
	}

	return dest.String(), nil
}

func getBaseIndex(path string) int {
	normalized := strings.ToLower(path)
	idx := strings.Index(normalized, "/planes/")
	if idx >= 0 {
		return idx
	}
	idx = strings.Index(normalized, "/subscriptions/")
	if idx >= 0 {
		return idx
	}
	return 0

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
	Body v1.ErrorResponse
}

// NewLinkedResourceUpdateErrorResponse represents a HTTP 400 with an error message when user updates environment id and application id.
func NewLinkedResourceUpdateErrorResponse(resourceID resources.ID, oldProp *rpv1.BasicResourceProperties, newProp *rpv1.BasicResourceProperties) Response {
	newAppEnv := ""
	if newProp.Application != "" {
		name := newProp.Application
		if rid, err := resources.ParseResource(newProp.Application); err == nil {
			name = rid.Name()
		}
		newAppEnv += fmt.Sprintf("'%s' application", name)
	}
	if newProp.Environment != "" {
		if newAppEnv != "" {
			newAppEnv += " and "
		}
		name := newProp.Environment
		if rid, err := resources.ParseResource(newProp.Environment); err == nil {
			name = rid.Name()
		}
		newAppEnv += fmt.Sprintf("'%s' environment", name)
	}

	message := fmt.Sprintf(LinkedResourceUpdateErrorFormat, resourceID.Name(), resourceID.Name(), newAppEnv, oldProp.Application, oldProp.Environment, resourceID.Name())
	return &BadRequestResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: message,
				Target:  resourceID.String(),
			},
		},
	}
}

func NewBadRequestResponse(message string) Response {
	return &BadRequestResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: message,
			},
		},
	}
}

func NewBadRequestARMResponse(body v1.ErrorResponse) Response {
	return &BadRequestResponse{
		Body: body,
	}
}

func (r *BadRequestResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusBadRequest), logging.LogHTTPStatusCode, http.StatusBadRequest)

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

// ValidationErrorResponse represents an HTTP 400 with validation errors in ARM error format.
type ValidationErrorResponse struct {
	Body v1.ErrorResponse
}

func NewValidationErrorResponse(errors validator.ValidationErrors) Response {
	body := v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: errors.Error(),
		},
	}

	for _, fe := range errors {
		if err, ok := fe.(error); ok {
			detail := v1.ErrorDetails{
				Target:  fe.Field(),
				Message: err.Error(),
			}
			body.Error.Details = append(body.Error.Details, detail)
		}
	}

	return &ValidationErrorResponse{Body: body}
}

func (r *ValidationErrorResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusBadRequest), logging.LogHTTPStatusCode, http.StatusBadRequest)

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
	Body v1.ErrorResponse
}

// NewNotFoundMessageResponse represents an HTTP 404 with string message.
func NewNotFoundMessageResponse(message string) Response {
	return &NotFoundResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeNotFound,
				Message: message,
			},
		},
	}
}

func NewNoResourceMatchResponse(path string) Response {
	return &NotFoundResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeNotFound,
				Message: fmt.Sprintf("the specified path %q did not match any resource", path),
				Target:  path,
			},
		},
	}
}

func NewNotFoundResponse(id resources.ID) Response {
	return &NotFoundResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeNotFound,
				Message: fmt.Sprintf("the resource with id '%s' was not found", id.String()),
				Target:  id.String(),
			},
		},
	}
}

// NewNotFoundAPIVersionResponse creates Response for unsupported api version. (message is consistent with ARM).
func NewNotFoundAPIVersionResponse(resourceType string, namespace string, apiVersion string) Response {
	return &NotFoundResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalidResourceType, // ARM uses "InvalidResourceType" code with 404 http code.
				Message: fmt.Sprintf("The resource type '%s' could not be found in the namespace '%s' for api version '%s'.", resourceType, namespace, apiVersion),
			},
		},
	}
}

func (r *NotFoundResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusNotFound), logging.LogHTTPStatusCode, http.StatusNotFound)

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
	Body v1.ErrorResponse
}

func NewConflictResponse(message string) Response {
	return &ConflictResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeConflict,
				Message: message,
			},
		},
	}
}

func (r *ConflictResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusConflict), logging.LogHTTPStatusCode, http.StatusConflict)

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
	Body v1.ErrorResponse
}

func NewInternalServerErrorARMResponse(body v1.ErrorResponse) Response {
	return &InternalServerErrorResponse{
		Body: body,
	}
}

func (r *InternalServerErrorResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusInternalServerError), logging.LogHTTPStatusCode, http.StatusInternalServerError)

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

// PreconditionFailedResponse represents an HTTP 412 with an ARM error payload.
type PreconditionFailedResponse struct {
	Body v1.ErrorResponse
}

func NewPreconditionFailedResponse(target string, message string) Response {
	return &PreconditionFailedResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodePreconditionFailed,
				Message: message,
				Target:  target,
			},
		},
	}
}

func (r *PreconditionFailedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusPreconditionFailed), logging.LogHTTPStatusCode, http.StatusPreconditionFailed)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusPreconditionFailed)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}

// ClientAuthenticationFailed represents an HTTP 401 with an ARM error payload.
type ClientAuthenticationFailed struct {
	Body v1.ErrorResponse
}

func NewClientAuthenticationFailedARMResponse() Response {
	return &ClientAuthenticationFailed{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalidAuthenticationInfo,
				Message: "Server failed to authenticate the request",
			},
		},
	}
}
func (r *ClientAuthenticationFailed) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusUnauthorized), logging.LogHTTPStatusCode, http.StatusUnauthorized)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}
	return nil
}

// AsyncOperationResultResponse
type AsyncOperationResultResponse struct {
	Headers map[string]string
}

func NewAsyncOperationResultResponse(headers map[string]string) Response {
	return &AsyncOperationResultResponse{
		Headers: headers,
	}
}

func (r *AsyncOperationResultResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusAccepted), logging.LogHTTPStatusCode, http.StatusAccepted)

	w.Header().Add("Content-Type", "application/json")

	for key, element := range r.Headers {
		w.Header().Add(key, element)
	}

	w.WriteHeader(http.StatusAccepted)

	return nil
}

// MethodNotAllowedResponse represents an HTTP 405 with an ARM error payload.
type MethodNotAllowedResponse struct {
	Body v1.ErrorResponse
}

// NewMethodNotAllowedResponse creates MethodNotAllowedResponse instance.
func NewMethodNotAllowedResponse(target string, message string) Response {
	return &MethodNotAllowedResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: message,
				Target:  target,
			},
		},
	}
}

func (r *MethodNotAllowedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", http.StatusMethodNotAllowed), logging.LogHTTPStatusCode, http.StatusMethodNotAllowed)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, err = w.Write(bytes)
	if err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %s", r.Body, err)
	}

	return nil
}
