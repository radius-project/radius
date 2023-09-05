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
	"net/http"
	"net/http/httputil"
	"net/url"
)

// DirectorFunc is a function that modifies the request before it is sent to the downstream server.
type DirectorFunc = func(r *http.Request)

// ResponderFunc is a function that modifies the response before it is sent to the client.
type ResponderFunc = func(r *http.Response) error

// ErrorHandlerFunc is a function that handles errors that occur during the request.
type ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error)

// ReverseProxy defines the interface for a reverse proxy.
type ReverseProxy interface {
	http.Handler
}

// ReverseProxyOptions defines the options for creating a reverse proxy.
type ReverseProxyOptions struct {
	// RoundTripper is the round tripper used by the reverse proxy to send requests.
	RoundTripper http.RoundTripper
}

type ReverseProxyBuilder struct {
	// Downstream is the URL of the downstream server. This is the URL of the destination.
	//
	// The downstream URL will replace the request URL's scheme, host, and port. If the downstream
	// URL contains a path, it will be pre-pended to the request URL's path.
	Downstream *url.URL

	// EnableLogging enables a set of logging middleware for the proxy.
	EnableLogging bool

	// Directors is the set of director functions to be applied to the reverse proxy.
	// Directors are applied in order and modify the request before it is sent to the downstream server.
	Directors []DirectorFunc

	// Responders is the set of responder functions to be applied to the reverse proxy.
	// Responses are applied in REVERSE order and modify the response before it is sent to the client.
	Responders []ResponderFunc

	// ErrorHandler is the error handler function to be applied to the reverse proxy.
	// The error handler is called when an Golang error. This is NOT called for HTTP errors such
	// as 404 or 500.
	ErrorHandler ErrorHandlerFunc

	// Transport is the transport set on the created httputil.ReverseProxy.
	Transport http.RoundTripper
}

// Build configures a ReverseProxy with the given parameters and returns a http.HandlerFunc.
func (builder *ReverseProxyBuilder) Build() ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(builder.Downstream)

	// NOTE: there's a built-in director. We prepend it here.
	//
	// We don't consider workaround28169 optional :-/ the default behavior is just broken.
	//
	// We don't want to propagate the Kubernetes authentication headers to the downstream server.
	directors := []DirectorFunc{rp.Director, workaround28169, filterKubernetesAPIServerHeaders}
	directors = append(directors, builder.Directors...)

	responders := builder.Responders

	errorHandler := defaultErrorHandler
	if builder.ErrorHandler != nil {
		errorHandler = builder.ErrorHandler
	}

	if builder.EnableLogging {
		// Insert handlers before AND after for logging.
		directors = append([]DirectorFunc{logUpstreamRequest}, directors...)
		directors = append(directors, logDownstreamRequest)

		responders = append([]ResponderFunc{logUpstreamResponse}, responders...)
		responders = append(responders, logDownstreamResponse)

		errorHandler = logConnectionError(errorHandler)
	}

	rp.Transport = builder.Transport
	rp.Director = director(directors)
	rp.ModifyResponse = responder(responders)
	rp.ErrorHandler = errorHandler

	return rp
}

func director(directors []DirectorFunc) DirectorFunc {
	return func(r *http.Request) {
		for _, director := range directors {
			director(r)
		}
	}
}

func responder(directors []ResponderFunc) ResponderFunc {
	return func(r *http.Response) error {
		for i := len(directors) - 1; i >= 0; i-- {
			err := directors[i](r)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

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
