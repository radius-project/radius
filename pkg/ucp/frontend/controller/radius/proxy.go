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

package radius

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/proxy"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	PlanesPath = "/planes"

	// ProcessOperationTimeout is the timeout for processing a tracked resource in the background.
	ProcessOperationTimeout = 12 * time.Hour

	// ProcessOperationRetryAfter is the retry interval for processing a tracked resource in the background.
	// This is used when the tracked resource is not in a terminal state.
	ProcessOperationRetryAfter = 5 * time.Second

	// EnqueueOperationRetryCount is the number of times to retry enqueueing an async operation before giving up.
	EnqueueOperationRetryCount = 10
)

type updater interface {
	Update(ctx context.Context, downstream string, id resources.ID, version string) error
}

var _ armrpc_controller.Controller = (*ProxyController)(nil)

// ProxyController is the controller implementation to proxy requests to appropriate RP or URL.
type ProxyController struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]

	// transport is the http.RoundTripper to use for proxying requests. Can be overridden for testing.
	transport http.RoundTripper

	// updater is used to process tracked resources. Can be overridden for testing.
	updater updater
}

// # Function Explanation
//
// NewProxyController creates a new ProxyPlane controller with the given options and returns it, or returns an error if the
// controller cannot be created.
func NewProxyController(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	transport := otelhttp.NewTransport(http.DefaultTransport)
	updater := trackedresource.NewUpdater(opts.StorageClient, &http.Client{Transport: transport})
	return &ProxyController{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.Plane]{}),
		transport: transport,
		updater:   updater,
	}, nil
}

// # Function Explanation
//
// Run() takes in a request object and context, looks up the plane and resource provider associated with the
// request, and proxies the request to the appropriate resource provider.
func (p *ProxyController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	logger.V(ucplog.LevelDebug).Info("starting proxy request")
	for key, value := range req.Header {
		logger.V(ucplog.LevelDebug).Info("incoming request header", "key", key, "value", value)
	}

	// NOTE: avoid using the request URL directly as the casing may have been normalized.
	// use the original URL instead.
	requestCtx := v1.ARMRequestContextFromContext(ctx)
	id := requestCtx.ResourceID
	relativePath := middleware.GetRelativePath(p.Options().PathBase, requestCtx.OrignalURL.Path)

	downstreamURL, err := resourcegroups.ValidateDownstream(ctx, p.StorageClient(), id)
	if errors.Is(err, &resourcegroups.NotFoundError{}) {
		return armrpc_rest.NewNotFoundResponse(id), nil
	} else if errors.Is(err, &resourcegroups.InvalidError{}) {
		response := v1.ErrorResponse{Error: v1.ErrorDetails{Code: v1.CodeInvalid, Message: err.Error(), Target: id.String()}}
		return armrpc_rest.NewBadRequestARMResponse(response), nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to validate downstream: %w", err)
	}

	proxyReq, err := p.PrepareProxyRequest(ctx, req, downstreamURL.String(), relativePath)
	if err != nil {
		return nil, err
	}

	interceptor := &responseInterceptor{Inner: p.transport}

	sender := proxy.NewARMProxy(proxy.ReverseProxyOptions{RoundTripper: interceptor}, downstreamURL, nil)
	sender.ServeHTTP(w, proxyReq)

	if interceptor.Response == nil {
		logger.V(ucplog.LevelDebug).Error(err, "failed to proxy request")
		return nil, nil
	}

	// If we get here then we've successfully proxied the request. Now we interpret the response.
	logger.V(ucplog.LevelDebug).Info("finished proxy request", "http.statuscode", interceptor.Response.StatusCode)
	for key, value := range req.Header {
		logger.V(ucplog.LevelDebug).Info("outgoing response header", "key", key, "value", value)
	}

	if !p.ShouldTrackRequest(req.Method, id, interceptor.Response) {
		logger.V(ucplog.LevelDebug).Info("request does not need to be tracked")
		return nil, nil
	}

	if p.IsTerminalResponse(interceptor.Response) {
		logger.V(ucplog.LevelDebug).Info("response is terminal, updating tracked resource synchronously")
		err = p.UpdateTrackedResource(ctx, downstreamURL.String(), id, requestCtx.APIVersion)
		if errors.Is(err, &trackedresource.InProgressErr{}) {
			logger.V(ucplog.LevelDebug).Info("synchronous update failed, updating tracked resource asynchronously")
			// Continue executing
		} else if err != nil {
			// We can't return the response to the client if we failed to update the tracked resource. Instead
			// fallback to the async path.
			logger.Error(err, "failed to update tracked resource synchronously")
			// Continue executing
		} else {
			logger.V(ucplog.LevelDebug).Info("tracked resource updated synchronously")
			return nil, nil
		}
	} else {
		logger.V(ucplog.LevelDebug).Info("response is not terminal, updating tracked resource asynchronously")
	}

	// If we get here then we need to update the tracked resource, but the operation is not yet complete.
	err = p.EnqueueTrackedResourceUpdate(ctx, id, requestCtx.APIVersion)
	if err != nil {
		logger.Error(err, "failed to enqueue tracked resource update")
		return nil, nil
	}

	return nil, nil
}

// PrepareProxyRequest constructs and initializes the proxy request.
func (p *ProxyController) PrepareProxyRequest(ctx context.Context, originalReq *http.Request, downstream string, relativePath string) (*http.Request, error) {
	proxyReq := originalReq.Clone(ctx)
	requestURL, err := url.Parse(downstream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse downstream URL: %w", err)
	}
	proxyReq.URL = requestURL
	proxyReq.URL.Path = relativePath
	proxyReq.URL.RawQuery = originalReq.URL.RawQuery

	refererURL := url.URL{
		Scheme:   "http",
		Host:     originalReq.Host,
		Path:     originalReq.URL.Path,
		RawQuery: originalReq.URL.RawQuery,
	}

	// As per https://github.com/golang/go/issues/28940#issuecomment-441749380, the way to check
	// for http vs https is check the TLS field
	if originalReq.TLS != nil {
		refererURL.Scheme = "https"
	}

	proxyReq.Header.Set("X-Forwarded-Proto", refererURL.Scheme)
	proxyReq.Header.Set(v1.RefererHeader, refererURL.String())

	return proxyReq, nil
}

// ShouldTrackRequest returns true if the request should be tracked.
func (p *ProxyController) ShouldTrackRequest(httpMethod string, id resources.ID, resp *http.Response) bool {
	// Only track mutating requests.
	if !strings.EqualFold(httpMethod, http.MethodPut) && !strings.EqualFold(httpMethod, http.MethodPatch) && !strings.EqualFold(httpMethod, http.MethodDelete) {
		return false
	}

	// For now we just track top-level resources.
	if len(id.TypeSegments()) != 1 || !id.IsResource() {
		return false
	}

	if resp.StatusCode < 200 && resp.StatusCode >= 300 {
		return false // Not a success
	}

	return true
}

// IsTerminalResponse returns true if the response is terminal.
func (p *ProxyController) IsTerminalResponse(resp *http.Response) bool {
	return resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted
}

// UpdateTrackedResource updates the tracked resource synchronously.
func (p *ProxyController) UpdateTrackedResource(ctx context.Context, downstream string, id resources.ID, apiVersion string) error {
	return p.updater.Update(ctx, downstream, id, apiVersion)
}

// EnqueueTrackedResourceUpdate enqueues an async operation to update the tracked resource.
func (p *ProxyController) EnqueueTrackedResourceUpdate(ctx context.Context, id resources.ID, apiVersion string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	trackingID := trackedresource.IDFor(id)

	// Create a serviceCtx for the operation that we're going to process on the resource.
	serviceCtx := *v1.ARMRequestContextFromContext(ctx)
	serviceCtx.ResourceID = trackingID
	serviceCtx.OperationType = v1.OperationType{Type: trackingID.Type(), Method: datamodel.OperationProcess}

	// Create the database entry for the tracked resource.
	//
	// If a non-terminal response was returned from the RP then at this instant the resource exists, even if it is
	// being deleted.
	entry := datamodel.GenericResourceFromID(id, trackingID)
	entry.Properties.APIVersion = apiVersion
	entry.Properties.OperationID = serviceCtx.OperationID.String()

	// We need to update the tracked resource entry in the database using optimistic concurrency. This means that we
	// need to read the existing entry, update it, and then write it back. If the write fails then we need to retry.
	//
	// This concurrency scheme ensures that the background process will "observe" the last state of the resource.
	//
	// Think of it like this, each time the resource is changing we poke the background process and say "hey, the
	// resource is changing, you should check it out". The background process then reads the resource and updates the
	// state.
	queueOperation := false
retry:
	for retryCount := 1; retryCount <= EnqueueOperationRetryCount; retryCount++ {
		obj, err := p.StorageClient().Get(ctx, trackingID.String())
		if errors.Is(err, &store.ErrNotFound{}) {
			// Safe to ignore. This means that the resource has not been tracked yet.
		} else if err != nil {
			return err
		}

		etag := ""
		if obj != nil {
			etag = obj.ETag
			err = obj.As(&entry)
			if err != nil {
				return err
			}
		}

		// Keep the existing provisioningState if possible.
		if entry.InternalMetadata.AsyncProvisioningState == "" || entry.InternalMetadata.AsyncProvisioningState.IsTerminal() {
			queueOperation = true
			entry.InternalMetadata.AsyncProvisioningState = v1.ProvisioningStateAccepted
		}

		logger.V(ucplog.LevelDebug).Info("enqueuing tracked resource update")
		err = p.StorageClient().Save(ctx, &store.Object{Metadata: store.Metadata{ID: trackingID.String()}, Data: entry}, store.WithETag(etag))
		if errors.Is(err, &store.ErrConcurrency{}) {
			// This means we hit a concurrency error saving the tracked resource entry. This means that the resource
			// was updated in the background. We should retry.
			logger.V(ucplog.LevelDebug).Info("enqueue tracked resource update failed due to concurrency error", "retryCount", retryCount)
			continue
		} else if err != nil {
			return err
		}

		break retry
	}

	// Only queue an operation if necessary, eg: if we changed the provisioningState.
	if !queueOperation {
		return nil
	}

	err := p.StatusManager().QueueAsyncOperation(ctx, &serviceCtx, statusmanager.QueueOperationOptions{OperationTimeout: ProcessOperationTimeout, RetryAfter: ProcessOperationRetryAfter})
	if err != nil {
		return err
	}

	return nil
}

// responseInterceptor is a http.RoundTripper that records the response and error from the inner http.RoundTripper.
//
// This type is NOT thread-safe and should be created and used per-request.
type responseInterceptor struct {
	Inner http.RoundTripper

	Response *http.Response
	Error    error
}

// RoundTrip implements http.RoundTripper by capturing the response and error.
func (i *responseInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	i.Response, i.Error = i.Inner.RoundTrip(req)
	return i.Response, i.Error
}
