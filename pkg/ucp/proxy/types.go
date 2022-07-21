// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

type UCPRequestInfo struct {
	PlaneURL   string
	PlaneKind  string
	PlaneID    string
	HTTPScheme string
	UCPHost    string
}

type PlaneUrlFieldType string
type PlaneIdFieldType string
type HttpSchemeType string
type UCPHostType string
type UCPRequestInfoFieldType string

const (
	LocationHeader                                    = "Location"
	AzureAsyncOperationHeader                         = "Azure-Asyncoperation"
	UCPRequestInfoField       UCPRequestInfoFieldType = "ucprequestinfo"
)

type DirectorFunc = func(r *http.Request)
type ResponderFunc = func(r *http.Response) error
type ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error)

type ReverseProxy interface {
	http.Handler
}

type ReverseProxyOptions struct {
	RoundTripper     http.RoundTripper
	ProxyAddress     string
	TrimPlanesPrefix bool
}

type ReverseProxyBuilder struct {
	Downstream    *url.URL
	EnableLogging bool
	Directors     []DirectorFunc
	Responders    []ResponderFunc
	ErrorHandler  ErrorHandlerFunc

	// Transport is the transport set on the created httputil.ReverseProxy.
	Transport Transport
}

type Transport struct {
	roundTripper http.RoundTripper
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.roundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (builder *ReverseProxyBuilder) Build() ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(builder.Downstream)
	rp.Transport = &builder.Transport

	// We don consider workaround28169 required :-/ the default behavior is just broken.
	//
	// There's always a default director so this is safe.
	rp.Director = appendDirector(rp.Director, workaround28169)
	rp.Director = appendDirector(rp.Director, builder.Directors...)

	// There's never a default responder.
	rp.ModifyResponse = appendResponder(noopResponder, builder.Responders...)

	rp.ErrorHandler = builder.ErrorHandler
	if rp.ErrorHandler == nil {
		rp.ErrorHandler = defaultErrorHandler
	}

	if builder.EnableLogging {
		// Insert handlers before AND after for logging.
		rp.Director = appendDirector(logUpstreamRequest, rp.Director, logDownstreamRequest)
		rp.ModifyResponse = appendResponder(logDownstreamResponse, rp.ModifyResponse, logUpstreamResponse)
		rp.ErrorHandler = appendErrorHandler(logConnectionError, rp.ErrorHandler)
	}

	return http.HandlerFunc(rp.ServeHTTP)
}

func (p *armProxy) processAsyncResponse(resp *http.Response) error {
	return nil
}
