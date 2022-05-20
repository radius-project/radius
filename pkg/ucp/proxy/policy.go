// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"net/http"

	"github.com/go-logr/logr"
)

func workaround28169(r *http.Request) {
	// See: https://github.com/golang/go/issues/28168
	//
	// The built-in support will get the Host header wrong, which is a big problem. Almost every
	// significant service validates its Host header.
	r.Host = r.URL.Host
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
}

func logDownstreamRequest(r *http.Request) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Info("sending proxy request to downstream", "url", r.URL.String(), "method", r.Method)
}

func logDownstreamResponse(r *http.Response) error {
	logger := logr.FromContextOrDiscard(r.Request.Context())
	logger.Info("received proxy response from downstream", "status", r.Status)

	return nil
}

func logUpstreamResponse(r *http.Response) error {
	logger := logr.FromContextOrDiscard(r.Request.Context())
	logger.Info("sending proxy response to upstream", "status", r.Status)

	return nil
}

func logConnectionError(w http.ResponseWriter, r *http.Request, err error) {
	logger := logr.FromContextOrDiscard(r.Context())
	logger.Error(err, "connection failed to downstream", "url", r.URL.String(), "method", r.Method)
}
