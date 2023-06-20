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
package planes

import (
	"context"
	"fmt"
	http "net/http"
	"net/url"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/proxy"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	PlanesPath = "/planes"
)

var _ armrpc_controller.Controller = (*ProxyPlane)(nil)

// ProxyPlane is the controller implementation to proxy requests to appropriate RP or URL.
type ProxyPlane struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewProxyPlane creates a new ProxyPlane.
func NewProxyPlane(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ProxyPlane{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.Plane]{}),
	}, nil
}

func (p *ProxyPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	logger.Info("starting proxy request")
	for key, value := range req.Header {
		logger.V(ucplog.Debug).Info("incoming request header", "key", key, "value", value)
	}

	refererURL := url.URL{
		Host:     req.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}

	// Make a copy of the incoming URL and trim the base path
	newURL := *req.URL
	newURL.Path = middleware.GetRelativePath(p.Options().PathBase, req.URL.Path)
	planeType, name, _, err := resources.ExtractPlanesPrefixFromURLPath(newURL.Path)
	if err != nil {
		return nil, err
	}

	// Lookup the plane
	planePath := PlanesPath + "/" + planeType + "/" + name
	planeID, err := resources.ParseScope(planePath)
	if err != nil {
		return nil, err
	}

	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	plane, _, err := p.GetResource(ctx, planeID)
	if err != nil {
		return nil, err
	}
	if plane == nil {
		restResponse := armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID)
		return restResponse, nil
	}

	if plane.Properties.Kind == rest.PlaneKindUCPNative {
		// Check if the resource group exists
		id, err := resources.Parse(newURL.Path)
		if err != nil {
			return nil, err
		}
		rgPath := id.RootScope()
		rgID, err := resources.ParseScope(rgPath)
		if err != nil {
			return nil, err
		}

		existingRG, _, err := p.GetResource(ctx, rgID)
		if err != nil {
			return nil, err
		}
		if existingRG == nil {
			logger.Info(fmt.Sprintf("Resource group %s not found in db", serviceCtx.ResourceID))
			restResponse := armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID)
			return restResponse, nil
		}
	}

	// Get the resource provider
	resourceID, err := resources.Parse(newURL.Path)
	if err != nil {
		return nil, err
	}

	// We expect either a resource or resource collection.
	if resourceID.ProviderNamespace() == "" {
		err = fmt.Errorf("invalid resourceID specified with no provider")
		logger.Error(err, "resourceID %q does not have provider", resourceID.String())
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	// Lookup the resource providers configured to determine the URL to proxy to
	// Not using map lookups to enable case insensitive comparisons
	// We need to preserve the case while storing data in DB and therefore iterating for case
	// insensitive comparisons

	var proxyURL string
	if plane.Properties.Kind == rest.PlaneKindUCPNative {
		proxyURL = plane.LookupResourceProvider(resourceID.ProviderNamespace())
		if proxyURL == "" {
			err = fmt.Errorf("provider %s not configured", resourceID.ProviderNamespace())
			return nil, err
		}
	} else {
		// For a non UCP-native plane, the configuration should have a URL to which
		// all the requests will be forwarded
		proxyURL = *plane.Properties.URL
	}

	downstream, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	options := proxy.ReverseProxyOptions{
		RoundTripper:     otelhttp.NewTransport(http.DefaultTransport),
		ProxyAddress:     p.Options().Address,
		TrimPlanesPrefix: (plane.Properties.Kind != rest.PlaneKindUCPNative),
	}

	// As per https://github.com/golang/go/issues/28940#issuecomment-441749380, the way to check
	// for http vs https is check the TLS field
	httpScheme := "http"
	if req.TLS != nil {
		httpScheme = "https"
	}
	refererURL.Scheme = httpScheme

	requestInfo := proxy.UCPRequestInfo{
		PlaneURL:   proxyURL,
		PlaneKind:  string(plane.Properties.Kind),
		PlaneID:    planePath,
		HTTPScheme: httpScheme,
		// The Host field in the request that the client makes to UCP contains the UCP Host address
		// That address will be used to construct the URL for reverse proxying
		UCPHost: req.Host + p.Options().PathBase,
	}

	uri, err := url.Parse(newURL.Path)
	if err != nil {
		return nil, err
	}

	// Preserving the query strings on the incoming url on the newly constructed url
	uri.RawQuery = newURL.Query().Encode()
	req.URL = uri
	req.Header.Set("X-Forwarded-Proto", httpScheme)

	req.Header.Set(v1.RefererHeader, refererURL.String())
	logger = ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Referer Header: %s", req.Header.Get(v1.RefererHeader)))

	ctx = context.WithValue(ctx, proxy.UCPRequestInfoField, requestInfo)
	sender := proxy.NewARMProxy(options, downstream, nil)

	logger.Info(fmt.Sprintf("proxying request target: %s", proxyURL))
	sender.ServeHTTP(w, req.WithContext(ctx))
	// The upstream response has already been sent at this point. Therefore, return nil response here
	return nil, nil
}
