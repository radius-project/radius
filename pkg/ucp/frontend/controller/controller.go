// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	sm "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Options represents controller options.
type Options struct {
	BasePath string
	// DB is the data storage client.
	DB           store.StorageClient
	SecretClient secret.Client
	Address      string

	// DataProvider is the data storage provider.
	DataProvider dataprovider.DataStorageProvider

	// ResourceType is the string that represents the resource type.
	ResourceType string

	// StatusManager
	StatusManager sm.StatusManager

	AWSCloudControlClient   ucp_aws.AWSCloudControlClient
	AWSCloudFormationClient ucp_aws.AWSCloudFormationClient

	// CommonControllerOptions is the set of options used by most of our controllers.
	//
	// TODO: over time we should replace Options with CommonControllerOptions.
	CommonControllerOptions armrpc_controller.Options
}

// ResourceOptions represents the options and filters for resource.
type ResourceOptions[T any] struct {
	// RequestConverter is the request converter.
	RequestConverter v1.ConvertToDataModel[T]

	// ResponseConverter is the response converter.
	ResponseConverter v1.ConvertToAPIModel[T]

	// DeleteFilters is a slice of filters that execute prior to deleting a resource.
	DeleteFilters []DeleteFilter[T]

	// UpdateFilters is a slice of filters that execute prior to updating a resource.
	UpdateFilters []UpdateFilter[T]

	// AsyncOperationTimeout is the default timeout duration of async put operation.
	AsyncOperationTimeout time.Duration
}

type ControllerFunc func(Options) (armrpc_controller.Controller, error)

type HandlerOptions struct {
	ParentRouter   *mux.Router
	ResourceType   string
	Path           string
	Method         v1.OperationMethod
	HandlerFactory ControllerFunc
}

// BaseController is the base operation controller.
type BaseController struct {
	Options Options
}

// NewBaseController creates BaseController instance.
func NewBaseController(options Options) BaseController {
	return BaseController{
		options,
	}
}

func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts Options) error {
	storageClient, err := ctrlOpts.CommonControllerOptions.DataProvider.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}
	ctrlOpts.CommonControllerOptions.StorageClient = storageClient
	ctrlOpts.CommonControllerOptions.ResourceType = opts.ResourceType

	ctrl, err := opts.HandlerFactory(ctrlOpts)
	if err != nil {
		return err
	}

	fn := func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		response, err := ctrl.Run(ctx, w, req)
		if err != nil {
			HandleError(ctx, w, req, err)
			return
		}
		if response != nil {
			err = response.Apply(ctx, w, req)
			if err != nil {
				HandleError(ctx, w, req, err)
				return
			}
		}
	}

	ot := v1.OperationType{Type: opts.Path, Method: opts.Method}
	if opts.Method != "" {
		opts.ParentRouter.Methods(opts.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())
	} else {
		// Path is used to proxy plane request irrespective of the http method
		opts.ParentRouter.PathPrefix(opts.Path).HandlerFunc(fn).Name(ot.String())
	}
	return nil
}

// StorageClient gets storage client for this controller.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.Options.DB
}

// GetResource is the helper to get the resource via storage client.
func (c *BaseController) GetResource(ctx context.Context, id string, out any) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = c.StorageClient().Get(ctx, id); err == nil && res != nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}

// SaveResource is the helper to save the resource via storage client.
func (c *BaseController) SaveResource(ctx context.Context, id string, in any, etag string) (*store.Object, error) {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := c.StorageClient().Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}

// DeleteResource is the helper to delete the resource via storage client.
func (c *BaseController) DeleteResource(ctx context.Context, id string, etag string) error {
	err := c.StorageClient().Delete(ctx, id, store.WithETag(etag))
	if err != nil {
		return err
	}
	return nil
}

// Responds with an HTTP 500
func HandleError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	var response armrpc_rest.Response
	// Try to use the ARM format to send back the error info
	// if the error is due to api conversion failure return bad request
	switch v := err.(type) {
	case *v1.ErrModelConversion:
		response = armrpc_rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
				Message: err.Error(),
			},
		})
	case *v1.ErrClientRP:
		response = armrpc_rest.NewBadRequestARMResponse(v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v.Code,
				Message: v.Message,
			},
		})
	default:
		if err.Error() == v1.ErrInvalidModelConversion.Error() {
			response = armrpc_rest.NewBadRequestARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
					Message: err.Error(),
				},
			})
		} else {
			logger.V(ucplog.Debug).Error(err, "unhandled error")
			response = armrpc_rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: err.Error(),
				},
			})
		}
	}
	err = response.Apply(ctx, w, req)
	if err != nil {
		body := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInternal,
				Message: err.Error(),
			},
		}
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}

func (b *BaseController) GetRelativePath(path string) string {
	trimmedPath := strings.TrimPrefix(path, b.Options.BasePath)
	return trimmedPath
}

func (b *BaseController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	path := b.GetRelativePath(r.URL.Path)
	restResponse := armrpc_rest.NewNoResourceMatchResponse(path)
	err := restResponse.Apply(r.Context(), w, r)
	if err != nil {
		HandleError(r.Context(), w, r, err)
		return
	}
}

func (b *BaseController) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	path := b.GetRelativePath(r.URL.Path)
	target := ""
	if rID, err := resources.Parse(path); err == nil {
		target = rID.Type() + "/" + rID.Name()
	}
	restResponse := armrpc_rest.NewMethodNotAllowedResponse(target, fmt.Sprintf("The request method '%s' is invalid.", r.Method))
	if err := restResponse.Apply(r.Context(), w, r); err != nil {
		HandleError(r.Context(), w, r, err)
	}
}

func ReadRequestBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}

func ConfigureDefaultHandlers(router *mux.Router, opts Options) {
	b := NewBaseController(opts)
	router.NotFoundHandler = http.HandlerFunc(b.NotFoundHandler)
	router.MethodNotAllowedHandler = http.HandlerFunc(b.MethodNotAllowedHandler)
}

// GetAPIVersion extracts the API version from the request
func GetAPIVersion(req *http.Request) string {
	return req.URL.Query().Get("api-version")
}

// DeleteFilter is a function that is executed as part of the controller lifecycle. DeleteFilters can be used to:
//
// - Block deletion of a resource based on some arbitrary condition.
//
// DeleteFilters should return a rest.Response to handle the request without allowing deletion to occur. Any
// errors returned will be treated as "unhandled" and logged before sending back an HTTP 500.
type DeleteFilter[T any] func(ctx context.Context, oldResource *T, options *Options) (rest.Response, error)

// UpdateFilter is a function that is executed as part of the controller lifecycle. UpdateFilters can be used to:
//
// - Set internal state of a resource data model prior to saving.
// - Perform semantic validation based on the old state of a resource.
// - Perform semantic validation based on external state.
//
// UpdateFilters should return a rest.Response to handle the request without allowing updates to occur. Any
// errors returned will be treated as "unhandled" and logged before sending back an HTTP 500.
type UpdateFilter[T any] func(ctx context.Context, newResource *T, oldResource *T, options *Options) (rest.Response, error)

var (
	// ContentTypeHeaderKey is the header key of Content-Type
	ContentTypeHeaderKey = http.CanonicalHeaderKey("Content-Type")

	// DefaultScheme is the default scheme used if there is no scheme in the URL.
	DefaultSheme = "http"
)

var (
	// ErrUnsupportedContentType represents the error of unsupported content-type.
	ErrUnsupportedContentType = errors.New("unsupported Content-Type")
	// ErrRequestedResourceDoesNotExist represents the error of resource that is requested not existing.
	ErrRequestedResourceDoesNotExist = errors.New("requested resource does not exist")
	// ErrETagsDoNotMatch represents the error of the eTag of the resource and the requested etag not matching.
	ErrETagsDoNotMatch = errors.New("etags do not match")
	// ErrResourceAlreadyExists represents the error of the resource being already existent at the moment.
	ErrResourceAlreadyExists = errors.New("resource already exists")
)

// ReadJSONBody extracts the content from request.
func ReadJSONBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get(ContentTypeHeaderKey)))
	if i := strings.Index(contentType, ";"); i > -1 {
		contentType = contentType[0:i]
	}

	if contentType != "application/json" {
		return nil, ErrUnsupportedContentType
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return data, nil
}

// ValidateETag receives an ARMRequestContect and gathers the values in the If-Match and/or
// If-None-Match headers and then checks to see if the etag of the resource matches what is requested.
func ValidateETag(armRequestContext v1.ARMRequestContext, etag string) error {
	ifMatchETag := armRequestContext.IfMatch
	ifMatchCheck := checkIfMatchHeader(ifMatchETag, etag)
	if ifMatchCheck != nil {
		return ifMatchCheck
	}

	ifNoneMatchETag := armRequestContext.IfNoneMatch
	ifNoneMatchCheck := checkIfNoneMatchHeader(ifNoneMatchETag, etag)
	if ifNoneMatchCheck != nil {
		return ifNoneMatchCheck
	}

	return nil
}

// checkIfMatchHeader function checks if the etag of the resource matches
// the one provided in the if-match header
func checkIfMatchHeader(ifMatchETag string, etag string) error {
	if ifMatchETag == "" {
		return nil
	}

	if etag == "" {
		return ErrRequestedResourceDoesNotExist
	}

	if ifMatchETag != "*" && ifMatchETag != etag {
		return ErrETagsDoNotMatch
	}

	return nil
}

// checkIfNoneMatchHeader function checks if the etag of the resource matches
// the one provided in the if-none-match header
func checkIfNoneMatchHeader(ifNoneMatchETag string, etag string) error {
	if ifNoneMatchETag == "*" && etag != "" {
		return ErrResourceAlreadyExists
	}

	return nil
}
