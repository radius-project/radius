// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type UCPRequestInfo struct {
	PlaneURL   string
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
	RoundTripper http.RoundTripper
	ProxyAddress string
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
	ctx := resp.Request.Context()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
		// As per https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/async-operations,
		// first check for Azure-AsyncOperation header and if not found, check for LocationHeader
		if azureAsyncOperationHeader, ok := resp.Header[AzureAsyncOperationHeader]; ok {
			// This is an Async Response with a Azure-AsyncOperation Header
			err := convertHeaderToUCPIDs(ctx, AzureAsyncOperationHeader, azureAsyncOperationHeader, resp)
			if err != nil {
				return err
			}
		} else if locationHeader, ok := resp.Header[LocationHeader]; ok {
			// This is an Async Response with a Location Header
			err := convertHeaderToUCPIDs(ctx, LocationHeader, locationHeader, resp)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func convertHeaderToUCPIDs(ctx context.Context, headerName string, header []string, resp *http.Response) error {
	segments := strings.Split(strings.TrimSuffix(strings.TrimPrefix(header[0], "/"), "/"), "/")
	// segment 0 -> http
	// segment 1 -> ""
	// segment 2 -> hostname + port
	key := segments[0] + "//" + segments[2]

	if ctx.Value(UCPRequestInfoField) == nil {
		return fmt.Errorf("Could not find ucp request data in %s header", headerName)
	}
	requestInfo := ctx.Value(UCPRequestInfoField).(UCPRequestInfo)
	fmt.Println(requestInfo)
	// Doing a reverse lookup of the URL of the responding server to find the corresponding plane ID
	if requestInfo.PlaneURL == "" {
		return fmt.Errorf("Could not find plane URL data in %s header", headerName)
	}
	if strings.TrimSuffix(requestInfo.PlaneURL, "/") != strings.TrimSuffix(key, "/") {
		return fmt.Errorf("PlaneURL: %s received in the request context does not match the url found in %s header", requestInfo.PlaneURL, headerName)
	}

	if requestInfo.UCPHost == "" {
		return fmt.Errorf("UCP Host Address unknown. Cannot convert response header")
	}
	// Make sure we only have the base URL here
	if requestInfo.PlaneID == "" {
		return fmt.Errorf("Could not find plane ID data in %s header", headerName)
	}

	if requestInfo.HTTPScheme == "" {
		return fmt.Errorf("Could not find http scheme data in %s header", headerName)
	}
	// Found a plane matching the URL in the location header
	// Convert to UCP ID using the planeID corresponding to the URL of the server from where the response was received
	val := requestInfo.HTTPScheme + "://" + requestInfo.UCPHost + requestInfo.PlaneID + "/" + strings.Join(segments[3:], "/")

	// Replace the header with the computed value.
	// Do not use the Del/Set methods on header as it can change the header casing to canonical form
	resp.Header[headerName] = []string{val}
	return nil
}
