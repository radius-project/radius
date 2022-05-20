// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"net/http"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func workaround28169(r *http.Request) {
	// See: https://github.com/golang/go/issues/28168
	//
	// The built-in support will get the Host header wrong, which is a big problem. Almost every
	// significant service validates its Host header.
	r.Host = r.URL.Host
}

func trimPlanesPrefix(r *http.Request) {
	_, _, remainder, err := resources.ExtractPlanesPrefixFromURLPath(r.URL.Path)
	if err != nil {
		// Invalid case like path: /planes/foo - do nothing
		// If we see an invalid URL here we don't have a good way to report an error at this point
		// we expect the error to have been handled before calling into this code.
		return
	}

	// Success -- truncate the planes prefix
	r.URL.Path = remainder
}

func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	w.WriteHeader(http.StatusBadGateway)
}

func noopResponder(r *http.Response) error {
	return nil
}

func logUpstreamRequest(r *http.Request) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Info("preparing proxy request for", "url", r.URL.String(), "method", r.Method)
	for key, value := range r.Header {
		logger.Info("incoming request header", "key", key, "value", value)
	}
}

func logDownstreamRequest(r *http.Request) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Info("sending proxy request to downstream", "url", r.URL.String(), "method", r.Method)
	for key, value := range r.Header {
		logger.Info("outgoing request header", "key", key, "value", value)
	}
}

func logDownstreamResponse(r *http.Response) error {
	logger := logr.FromContextOrDiscard(r.Request.Context())
	logger.Info("received proxy response from downstream", "status", r.Status)
	for key, value := range r.Request.Header {
		logger.Info("incoming response header", "key", key, "value", value)
	}

	return nil
}

func logUpstreamResponse(r *http.Response) error {
	logger := logr.FromContextOrDiscard(r.Request.Context())
	logger.Info("sending proxy response to upstream", "status", r.Status)
	for key, value := range r.Request.Header {
		logger.Info("outgoing response header", "key", key, "value", value)
	}

	return nil
}

func logConnectionError(w http.ResponseWriter, r *http.Request, err error) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Error(err, "connection failed to downstream", "url", r.URL.String(), "method", r.Method)
}
