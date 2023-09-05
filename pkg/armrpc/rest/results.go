/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/logging"

	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
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

// Apply sets the response headers and body, and writes the response to the http.ResponseWriter with a status
// code of 200. If an error occurs while marshaling the body or writing the response, an error is returned.
func (r *OKResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewCreatedResponse creates a Created HTTP Response object with the given data.
func NewCreatedResponse(body any) Response {
	response := &CreatedResponse{Body: body}
	return response
}

// Apply renders CreatedResponse to http.ResponseWriter by serializing empty body and set
// 201 Created response code and returns an error if any of these steps fail.
func (r *CreatedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewCreatedAsyncResponse creates a new HTTP Response for asynchronous operation.
func NewCreatedAsyncResponse(body any, location string, scheme string) Response {
	return &CreatedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

// Apply renders Created HTTP Response into http.ResponseWriter with Location header for asynchronous operation and returns an error if it fails.
func (r *CreatedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// Apply renders Accepted HTTP Response into http.ResponseWriter with Location header for asynchronous operation and returns an error if it fails.
func (r *AcceptedAsyncResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

	// RetryAfter is the value of the Retry-After header in seconds (as a string). This determines the client's polling interval.
	// Defaults to v1.DefaultRetryAfter. Consider setting a smaller value if your operation is expected to complete quickly.
	RetryAfter time.Duration
}

// NewAsyncOperationResponse creates an AsyncOperationResponse
func NewAsyncOperationResponse(body any, location string, code int, resourceID resources.ID, operationID uuid.UUID, apiVersion string, rootScope string, pathBase string) *AsyncOperationResponse {
	return &AsyncOperationResponse{
		Body:        body,
		Location:    location,
		Code:        code,
		ResourceID:  resourceID,
		OperationID: operationID,
		APIVersion:  apiVersion,
		RootScope:   rootScope,
		PathBase:    pathBase,
		RetryAfter:  v1.DefaultRetryAfterDuration,
	}
}

// Apply renders asynchronous operationStatuses response with Location/Azure-AsyncOperation URL headers and Retry-After header, which allows client to retry.
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
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Location", locationHeader)
	w.Header().Add("Azure-AsyncOperation", azureAsyncOpHeader)
	w.Header().Add("Retry-After", fmt.Sprintf("%v", r.RetryAfter.Truncate(time.Second).Seconds()))

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

	refererUrl := req.Header.Get(v1.RefererHeader)
	if refererUrl == "" {
		refererUrl = req.URL.String()
	}

	referer, err := url.Parse(refererUrl)
	if err != nil {
		return "", err
	}
	if referer.Host == "" {
		// Certain AWS requests don't forward the scheme/host
		// This case is to backfill the host for those AWS integration test requests
		referer.Host = req.Host
	}

	base := v1.ParsePathBase(referer.Path)
	dest := url.URL{
		Host:   referer.Host,
		Scheme: referer.Scheme,
		Path:   fmt.Sprintf("%s%s/providers/%s/locations/%s/%s/%s", base, rootScope, r.ResourceID.ProviderNamespace(), r.Location, resourceType, r.OperationID.String()),
	}

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

// NoContentResponse represents an HTTP 204.
//
// This is used for delete operations.
type NoContentResponse struct {
}

// NewNoContentResponse creates a new NoContentResponse object.
func NewNoContentResponse() Response {
	return &NoContentResponse{}
}

// Apply renders NoContent HTTP Response into http.ResponseWriter.
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

// NewDependencyMissingResponse creates a DependencyMissingResponse with a given error message.
func NewDependencyMissingResponse(message string) Response {
	return &BadRequestResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeDependencyMissing,
				Message: message,
			},
		},
	}
}

// NewBadRequestResponse creates a BadRequestResponse with a given error message.
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

// NewBadRequestARMResponse creates a BadRequestResponse with error message.
func NewBadRequestARMResponse(body v1.ErrorResponse) Response {
	return &BadRequestResponse{
		Body: body,
	}
}

// Apply renders the general BadRequest HTTP response into http.ResponseWriter by serializing ErrorResponse.
func (r *BadRequestResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewValidationErrorResponse creates a BadRequest response for invalid API validation.
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

// Apply renders BadRequest HTTP response into http.ResponseWriter by serializing invalid API validation error response and setting Content-Type.
func (r *ValidationErrorResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewNotFoundResponse creates a NotFoundResponse with resource id.
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

// Apply renders 404 NotFound HTTP response into http.ResponseWriter by setting Content-Type and serializing response.
func (r *NotFoundResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewConflictResponse creates a ConflictResponse for conflicting operations and resources.
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

// Apply renders 409 Conflict HTTP response into http.ResponseWriter by setting Content-Type and serializing response.
func (r *ConflictResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewInternalServerErrorARMResponse creates a new InternalServerErrorResponse with the given error message.
func NewInternalServerErrorARMResponse(body v1.ErrorResponse) Response {
	return &InternalServerErrorResponse{
		Body: body,
	}
}

// Apply renders 500 InternalServerError HTTP response into http.ResponseWriter by setting Content-Type and serializing response.
func (r *InternalServerErrorResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewPreconditionFailedResponse creates a new PreconditionFailedResponse with the given target resource and message.
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

// Apply renders 412 PreconditionFailed HTTP response into http.ResponseWriter by setting Content-Type and serializing response.
func (r *PreconditionFailedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewClientAuthenticationFailedARMResponse creates a ClientAuthenticationFailed Response with CodeInvalidAuthenticationInfo code and its message.
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

// Apply writes a response with status code 401 Unauthorized and a JSON body to the response writer. It returns an error
// if there is an issue marshaling the body or writing it to the response writer.
func (r *ClientAuthenticationFailed) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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

// NewAsyncOperationResultResponse creates a new AsyncOperationResultResponse with the given headers.
func NewAsyncOperationResultResponse(headers map[string]string) Response {
	return &AsyncOperationResultResponse{
		Headers: headers,
	}
}

// Apply sets the response headers and status code to http.StatusAccepted and returns nil.
func (r *AsyncOperationResultResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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
//

// NewMethodNotAllowedResponse creates a MethodNotAllowedResponse with the given message and target resource.
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

// Apply renders a HTTP response by serializing Body in JSON and setting 405 response code and returns an error if it fails.
func (r *MethodNotAllowedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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
