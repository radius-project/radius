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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/logging"

	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
//
// # Function Explanation
// 
//	NewOKResponse creates a new OKResponse object with the given body and returns it, allowing callers to handle errors by 
//	checking the response type.
func NewOKResponse(body any) Response {
	return &OKResponse{Body: body}
}

// NewOKResponseWithHeaders creates an OKResponse that will write a 200 OK with the provided body as JSON.
// Set the body to nil to write an empty 200 OK.
//
// # Function Explanation
// 
//	NewOKResponseWithHeaders creates a new OKResponse object with the given body and headers, and returns it to the caller. 
//	If any errors occur, an error is returned instead.
func NewOKResponseWithHeaders(body any, headers map[string]string) Response {
	return &OKResponse{
		Body:    body,
		Headers: headers,
	}
}

// # Function Explanation
// 
//	OKResponse.Apply writes the given headers and body to the response writer, logging the status code and returning any 
//	errors encountered. If an error occurs, the caller should check the error message for more information.
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

// # Function Explanation
// 
//	NewCreatedResponse creates a new CreatedResponse object with the given body and returns it. If an error occurs, it will 
//	be returned to the caller.
func NewCreatedResponse(body any) Response {
	response := &CreatedResponse{Body: body}
	return response
}

// # Function Explanation
// 
//	CreatedResponse.Apply writes a response with status code 201 (Created) to the given http.ResponseWriter, marshaling the 
//	given Body object to JSON and adding the Content-Type header. It returns an error if any of the steps fail.
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

// # Function Explanation
// 
//	This function, NewCreatedAsyncResponse, creates a new CreatedAsyncResponse object with the given body, location, and 
//	scheme, and returns it as a Response. It also handles any errors that may occur during the creation of the object.
func NewCreatedAsyncResponse(body any, location string, scheme string) Response {
	return &CreatedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

// # Function Explanation
// 
//	CreatedAsyncResponse.Apply writes a response to the given http.ResponseWriter with a status code of http.StatusCreated, 
//	marshals the given Body into JSON, adds the Location header to the response, and writes the marshaled JSON bytes to the 
//	output. If any errors occur during marshaling or writing, an error is returned.
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
//
// # Function Explanation
// 
//	NewAcceptedAsyncResponse creates a new AcceptedAsyncResponse object with the given body, location and scheme, and 
//	returns it to the caller. If any of the parameters are invalid, an error is returned.
func NewAcceptedAsyncResponse(body any, location string, scheme string) Response {
	return &AcceptedAsyncResponse{Body: body, Location: location, Scheme: scheme}
}

// # Function Explanation
// 
//	AcceptedAsyncResponse's Apply function takes in a context, a response writer and a request object and responds with an 
//	HTTP status code of 202 Accepted. It also sets the response body to a marshaled version of the Body field and sets the 
//	Location header to the Location field. If any errors occur during marshaling or writing, an error is returned.
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
//
// # Function Explanation
// 
//	NewAsyncOperationResponse creates a new AsyncOperationResponse object with the given parameters and a default 
//	RetryAfterDuration. It returns an error if any of the parameters are invalid.
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

// # Function Explanation
// 
//	AsyncOperationResponse.Apply writes the response body, headers and status code to the http.ResponseWriter, including the
//	 Location and Azure-AsyncOperation headers, and the Retry-After header with the truncated RetryAfter value in seconds. 
//	It returns an error if any of the operations fail.
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

// # Function Explanation
// 
//	NoContentResponse is a function that creates and returns a Response object with no content, handling any errors that may
//	 occur.
func NewNoContentResponse() Response {
	return &NoContentResponse{}
}

// # Function Explanation
// 
//	NoContentResponse.Apply writes a status code of 204 to the response writer and returns nil, indicating no error. If an 
//	error occurs, it will be returned to the caller.
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
//
// # Function Explanation
// 
//	NewLinkedResourceUpdateErrorResponse creates a BadRequestResponse with an error message that describes the issue when a 
//	linked resource cannot be updated due to a conflict between the new and old application/environment values.
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

// # Function Explanation
// 
//	NewBadRequestResponse creates a BadRequestResponse object with a given error message, which can be used to return an 
//	error response to the caller.
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

// # Function Explanation
// 
//	NewBadRequestARMResponse creates a BadRequestResponse object with the given body and returns it, allowing callers to 
//	handle errors in a consistent way.
func NewBadRequestARMResponse(body v1.ErrorResponse) Response {
	return &BadRequestResponse{
		Body: body,
	}
}

// # Function Explanation
// 
//	BadRequestResponse.Apply logs an info message with the status code, marshals the body into JSON, adds a content-type 
//	header, sets the status code to 400, and writes the marshaled body to the response writer. If any of these steps fail, 
//	an error is returned.
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

// # Function Explanation
// 
//	NewValidationErrorResponse creates a ValidationErrorResponse from a validator.ValidationErrors object, containing an 
//	ErrorResponse body with an ErrorDetails object and a list of ErrorDetails objects for each field error. It is used to 
//	provide callers with a structured response for validation errors.
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

// # Function Explanation
// 
//	ValidationErrorResponse.Apply logs the status code, marshals the body into JSON, adds the content type header, sets the 
//	status code to 400, and writes the marshaled body to the response. It returns an error if any of these steps fail.
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
//
// # Function Explanation
// 
//	"NewNotFoundMessageResponse" creates a new NotFoundResponse object with a given message and returns it as a Response. 
//	This response is used to indicate that the requested resource was not found and contains an error code and message to 
//	help the caller understand the cause of the error.
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

// # Function Explanation
// 
//	NewNoResourceMatchResponse creates a NotFoundResponse with an error message indicating that the specified path did not 
//	match any resource, and returns it. This response can be used by callers to handle the error.
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

// # Function Explanation
// 
//	NewNotFoundResponse creates a NotFoundResponse object with an error message containing the provided ID, which can be 
//	used to inform callers that the requested resource was not found.
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
//
// # Function Explanation
// 
//	NewNotFoundAPIVersionResponse creates an error response when a resource type, namespace, and API version are not found. 
//	It returns an error code and message to the caller, allowing them to handle the error accordingly.
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

// # Function Explanation
// 
//	NotFoundResponse.Apply logs the status code, marshals the body into JSON, adds the content type header, sets the status 
//	code to 404, and writes the marshaled body to the response. It returns an error if any of these steps fail.
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

// # Function Explanation
// 
//	NewConflictResponse creates a ConflictResponse object with a given message and returns it as a Response. It is used to 
//	handle errors and provide a response with an appropriate error code.
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

// # Function Explanation
// 
//	ConflictResponse.Apply takes in a context, a response writer and a request object and responds with a status code of 409
//	 Conflict and a JSON-formatted body. It logs the status code and handles any errors that occur while marshaling and 
//	writing the response.
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

// # Function Explanation
// 
//	NewInternalServerErrorARMResponse creates a new InternalServerErrorResponse object with the given body, which can be 
//	used to handle an internal server error when calling the function.
func NewInternalServerErrorARMResponse(body v1.ErrorResponse) Response {
	return &InternalServerErrorResponse{
		Body: body,
	}
}

// # Function Explanation
// 
//	InternalServerErrorResponse.Apply logs the status code, marshals the response body into JSON, adds the content type 
//	header, sets the status code to 500, and writes the marshaled response body to the output. If any of these steps fail, 
//	an error is returned to the caller.
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

// # Function Explanation
// 
//	NewPreconditionFailedResponse creates a new PreconditionFailedResponse object with a body containing an error response 
//	with the given target and message, which can be used to indicate a precondition failure to the caller.
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

// # Function Explanation
// 
//	PreconditionFailedResponse.Apply logs the status code, marshals the response body into JSON, adds the Content-Type 
//	header, writes the status code to the response, and writes the marshaled JSON to the response. If any of these steps 
//	fail, an error is returned.
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

// # Function Explanation
// 
//	NewClientAuthenticationFailedARMResponse() creates a ClientAuthenticationFailed response object with an error code and 
//	message, which can be used to indicate authentication failure to the caller.
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
// # Function Explanation
// 
//	ClientAuthenticationFailed applies the response for an authentication failure to the given context, request and response
//	 writer. It logs the status code, marshals the body into JSON, adds the content type to the response header and writes 
//	the response with the status code of 401 Unauthorized. If any of these steps fail, an error is returned.
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

// # Function Explanation
// 
//	NewAsyncOperationResultResponse creates a new AsyncOperationResultResponse object with the given headers and returns it,
//	 handling any errors that may occur.
func NewAsyncOperationResultResponse(headers map[string]string) Response {
	return &AsyncOperationResultResponse{
		Headers: headers,
	}
}

// # Function Explanation
// 
//	AsyncOperationResultResponse.Apply sends an HTTP response with status code 202 (Accepted) and the specified headers. It 
//	also logs the status code and returns any errors encountered.
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
// # Function Explanation
// 
//	NewMethodNotAllowedResponse creates a MethodNotAllowedResponse object with an ErrorResponse body containing an 
//	ErrorDetails object with the provided target and message, which can be used to handle errors.
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

// # Function Explanation
// 
//	MethodNotAllowedResponse.Apply logs the status code, marshals the response body into JSON, adds the Content-Type header,
//	 writes the status code to the response, and writes the marshaled JSON to the response. If any of these steps fail, an 
//	error is returned.
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
