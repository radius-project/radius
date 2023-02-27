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

	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
	// Doing a reverse lookup of the URL of the responding server to find the corresponding plane ID
	if requestInfo.PlaneURL == "" {
		return fmt.Errorf("Could not find plane URL data in %s header", headerName)
	}

	// Match the Plane URL but without the HTTP Scheme since the RP can return a https location/azure-asyncoperation header
	// based on the protocol scheme
	requestInfoPlaneID := strings.TrimSuffix(strings.Split(requestInfo.PlaneURL, "//")[1], "/")
	headerPlaneID := strings.TrimSuffix(strings.Split(key, "//")[1], "/")
	if !strings.EqualFold(requestInfoPlaneID, headerPlaneID) {
		return fmt.Errorf("PlaneURL: %s received in the request context does not match the url found in %s header: %s", requestInfo.PlaneURL, headerName, header[0])
	}

	if requestInfo.UCPHost == "" {
		return fmt.Errorf("UCP Host Address unknown. Cannot convert response header")
	}

	if requestInfo.PlaneKind == "" {
		return fmt.Errorf("Plane Kind unknown. Cannot convert response header")
	}

	var planeID string
	if requestInfo.PlaneKind != rest.PlaneKindUCPNative {
		if requestInfo.PlaneID == "" {
			return fmt.Errorf("Could not find plane ID data in %s header", headerName)
		}
		// Doing this only for non UCP Native planes. For UCP Native planes, the request URL will have the plane ID in it and therefore no need to
		// add the plane ID
		planeID = requestInfo.PlaneID
	}

	if requestInfo.HTTPScheme == "" {
		return fmt.Errorf("Could not find http scheme data in %s header", headerName)
	}

	// Found a plane matching the URL in the location header
	// Convert to UCP ID using the planeID corresponding to the URL of the server from where the response was received
	val := requestInfo.HTTPScheme + "://" + requestInfo.UCPHost + planeID + "/" + strings.Join(segments[3:], "/")

	// Replace the header with the computed value.
	// Do not use the Del/Set methods on header as it can change the header casing to canonical form
	resp.Header[headerName] = []string{val}

	logger := ucplog.FromContextWithSpan(ctx)
	logger.Info(fmt.Sprintf("Converting %s header from %s to %s", headerName, header[0], val))
	return nil
}
