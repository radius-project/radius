// Licensed under the Apache License, Version 2.0 . See LICENSE in the repository root for license information.
// Code generated by Microsoft (R) AutoRest Code Generator. DO NOT EDIT.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

package fake

import (
	"errors"
	"fmt"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake/server"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"net/http"
)

// PlanesServer is a fake server for instances of the v20231001preview.PlanesClient type.
type PlanesServer struct{
	// NewListPlanesPager is the fake for method PlanesClient.NewListPlanesPager
	// HTTP status codes to indicate success: http.StatusOK
	NewListPlanesPager func(options *v20231001preview.PlanesClientListPlanesOptions) (resp azfake.PagerResponder[v20231001preview.PlanesClientListPlanesResponse])

}

// NewPlanesServerTransport creates a new instance of PlanesServerTransport with the provided implementation.
// The returned PlanesServerTransport instance is connected to an instance of v20231001preview.PlanesClient via the
// azcore.ClientOptions.Transporter field in the client's constructor parameters.
func NewPlanesServerTransport(srv *PlanesServer) *PlanesServerTransport {
	return &PlanesServerTransport{
		srv: srv,
		newListPlanesPager: newTracker[azfake.PagerResponder[v20231001preview.PlanesClientListPlanesResponse]](),
	}
}

// PlanesServerTransport connects instances of v20231001preview.PlanesClient to instances of PlanesServer.
// Don't use this type directly, use NewPlanesServerTransport instead.
type PlanesServerTransport struct {
	srv *PlanesServer
	newListPlanesPager *tracker[azfake.PagerResponder[v20231001preview.PlanesClientListPlanesResponse]]
}

// Do implements the policy.Transporter interface for PlanesServerTransport.
func (p *PlanesServerTransport) Do(req *http.Request) (*http.Response, error) {
	rawMethod := req.Context().Value(runtime.CtxAPINameKey{})
	method, ok := rawMethod.(string)
	if !ok {
		return nil, nonRetriableError{errors.New("unable to dispatch request, missing value for CtxAPINameKey")}
	}

	return p.dispatchToMethodFake(req, method)
}

func (p *PlanesServerTransport) dispatchToMethodFake(req *http.Request, method string) (*http.Response, error) {
	resultChan := make(chan result)
	defer close(resultChan)

	go func() {
		var intercepted bool
		var res result
		 if planesServerTransportInterceptor != nil {
			 res.resp, res.err, intercepted = planesServerTransportInterceptor.Do(req)
		}
		if !intercepted {
			switch method {
			case "PlanesClient.NewListPlanesPager":
				res.resp, res.err = p.dispatchNewListPlanesPager(req)
				default:
		res.err = fmt.Errorf("unhandled API %s", method)
			}

		}
		select {
		case resultChan <- res:
		case <-req.Context().Done():
		}
	}()

	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case res := <-resultChan:
		return res.resp, res.err
	}
}

func (p *PlanesServerTransport) dispatchNewListPlanesPager(req *http.Request) (*http.Response, error) {
	if p.srv.NewListPlanesPager == nil {
		return nil, &nonRetriableError{errors.New("fake for method NewListPlanesPager not implemented")}
	}
	newListPlanesPager := p.newListPlanesPager.get(req)
	if newListPlanesPager == nil {
resp := p.srv.NewListPlanesPager(nil)
		newListPlanesPager = &resp
		p.newListPlanesPager.add(req, newListPlanesPager)
		server.PagerResponderInjectNextLinks(newListPlanesPager, req, func(page *v20231001preview.PlanesClientListPlanesResponse, createLink func() string) {
			page.NextLink = to.Ptr(createLink())
		})
	}
	resp, err := server.PagerResponderNext(newListPlanesPager, req)
	if err != nil {
		return nil, err
	}
	if !contains([]int{http.StatusOK}, resp.StatusCode) {
		p.newListPlanesPager.remove(req)
		return nil, &nonRetriableError{fmt.Errorf("unexpected status code %d. acceptable values are http.StatusOK", resp.StatusCode)}
	}
	if !server.PagerResponderMore(newListPlanesPager) {
		p.newListPlanesPager.remove(req)
	}
	return resp, nil
}

// set this to conditionally intercept incoming requests to PlanesServerTransport
var planesServerTransportInterceptor interface {
	// Do returns true if the server transport should use the returned response/error
	Do(*http.Request) (*http.Response, error, bool)
}