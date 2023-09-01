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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/proxy"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/rest"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	PlanesPath = "/planes"
)

var _ armrpc_controller.Controller = (*ProxyController)(nil)

// ProxyController is the controller implementation to proxy requests to appropriate RP or URL.
type ProxyController struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewProxyController creates a new ProxyPlane controller with the given options and returns it, or returns an error if the
// controller cannot be created.
func NewProxyController(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ProxyController{
		Operation: armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.Plane]{}),
	}, nil
}

// Run() takes in a request object and context, looks up the plane and resource provider associated with the
// request, and proxies the request to the appropriate resource provider.
func (p *ProxyController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	logger.Info("starting proxy request")
	for key, value := range req.Header {
		logger.V(ucplog.LevelDebug).Info("incoming request header", "key", key, "value", value)
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
		RoundTripper: otelhttp.NewTransport(http.DefaultTransport),
	}

	refererURL := url.URL{
		Scheme:   "http",
		Host:     req.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}

	// As per https://github.com/golang/go/issues/28940#issuecomment-441749380, the way to check
	// for http vs https is check the TLS field
	if req.TLS != nil {
		refererURL.Scheme = "https"
	}

	uri, err := url.Parse(newURL.Path)
	if err != nil {
		return nil, err
	}

	// Preserving the query strings on the incoming url on the newly constructed url
	uri.RawQuery = newURL.Query().Encode()
	req.URL = uri
	req.Header.Set("X-Forwarded-Proto", refererURL.Scheme)

	logger.Info("setting referer header", "value", refererURL.String())
	req.Header.Set(v1.RefererHeader, refererURL.String())

	sender := proxy.NewARMProxy(options, downstream, func(builder *proxy.ReverseProxyBuilder) {
		if plane.Properties.Kind != rest.PlaneKindUCPNative {
			// If we're proxying to Azure then remove the planes prefix.
			builder.Directors = append(builder.Directors, trimPlanesPrefix)
		}
	})

	logger.Info(fmt.Sprintf("proxying request target: %s", proxyURL))
	sender.ServeHTTP(w, req.WithContext(ctx))
	// The upstream response has already been sent at this point. Therefore, return nil response here
	return nil, nil
}

// trimPlanesPrefix trims the planes prefix from the request URL path.
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
