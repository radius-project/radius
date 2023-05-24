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

package proxy

import (
	"fmt"
	"net/http"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
	logger := ucplog.FromContextOrDiscard(r.Context())
	logger.Info("preparing proxy request")
}

func logDownstreamRequest(r *http.Request) {
	logger := ucplog.FromContextOrDiscard(r.Context())
	logger.Info("sending proxy request to downstream")
}

func logDownstreamResponse(r *http.Response) error {
	logger := ucplog.FromContextOrDiscard(r.Request.Context())
	logger.Info(fmt.Sprintf("received proxy response HTTP status code from downstream %d", r.StatusCode))
	return nil
}

func logUpstreamResponse(r *http.Response) error {
	logger := ucplog.FromContextOrDiscard(r.Request.Context())
	logger.Info(fmt.Sprintf("sending proxy response %d to upstream ", r.StatusCode))
	return nil
}

func logConnectionError(w http.ResponseWriter, r *http.Request, err error) {
	logger := ucplog.FromContextOrDiscard(r.Context())
	logger.Error(err, "connection failed to downstream")
}
