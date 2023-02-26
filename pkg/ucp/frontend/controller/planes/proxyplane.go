// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"errors"
	"fmt"
	http "net/http"
	"net/url"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/proxy"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const PlanesPath = "/planes"

var _ armrpc_controller.Controller = (*ProxyPlane)(nil)

// ProxyPlane is the controller implementation to proxy requests to appropriate RP or URL.
type ProxyPlane struct {
	ctrl.BaseController
}

// NewProxyPlane creates a new ProxyPlane.
func NewProxyPlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ProxyPlane{ctrl.NewBaseController(opts)}, nil
}

func (p *ProxyPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContext(ctx)

	logger.Info("starting proxy request")
	for key, value := range req.Header {
		logger.V(ucplog.Debug).Info("incoming request header", ucplog.Attributes("key", key, "value", value))
	}

	req.URL.Path = p.GetRelativePath(req.URL.Path)

	// Make a copy of the incoming URL and trim the base path
	newURL := *req.URL
	newURL.Path = p.GetRelativePath(req.URL.Path)
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

	plane := rest.Plane{}
	_, err = p.GetResource(ctx, planeID.String(), &plane)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			logger.Error(err, "plane %q does not exist", planeID.String())
			return armrpc_rest.NewNotFoundResponse(planeID), nil
		}
		return nil, err
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

		existingRG := datamodel.ResourceGroup{}
		_, err = p.GetResource(ctx, rgID.String(), &existingRG)
		if err != nil {
			if errors.Is(err, &store.ErrNotFound{}) {
				logger.Error(err, fmt.Sprintf("resource group %q does not exist", rgID.String()))
				return armrpc_rest.NewNotFoundResponse(rgID), nil
			}
			return nil, err
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
		proxyURL = plane.Properties.URL
	}

	downstream, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	options := proxy.ReverseProxyOptions{
		RoundTripper:     otelhttp.NewTransport(http.DefaultTransport),
		ProxyAddress:     p.Options.Address,
		TrimPlanesPrefix: (plane.Properties.Kind != rest.PlaneKindUCPNative),
	}

	// As per https://github.com/golang/go/issues/28940#issuecomment-441749380, the way to check
	// for http vs https is check the TLS field
	httpScheme := "http"
	if req.TLS != nil {
		httpScheme = "https"
	}

	requestInfo := proxy.UCPRequestInfo{
		PlaneURL:   proxyURL,
		PlaneKind:  string(plane.Properties.Kind),
		PlaneID:    planePath,
		HTTPScheme: httpScheme,
		// The Host field in the request that the client makes to UCP contains the UCP Host address
		// That address will be used to construct the URL for reverse proxying
		UCPHost: req.Host + p.Options.BasePath,
	}

	url, err := url.Parse(newURL.Path)
	if err != nil {
		return nil, err
	}

	// Preserving the query strings on the incoming url on the newly constructed url
	url.RawQuery = newURL.Query().Encode()
	req.URL = url
	req.Header.Set("X-Forwarded-Proto", httpScheme)

	ctx = context.WithValue(ctx, proxy.UCPRequestInfoField, requestInfo)
	sender := proxy.NewARMProxy(options, downstream, nil)

	logger = ucplog.FromContext(ctx)
	logger.Info(fmt.Sprintf("proxying request target: %s", proxyURL))
	sender.ServeHTTP(w, req.WithContext(ctx))
	// The upstream response has already been sent at this point. Therefore, return nil response here
	return nil, nil
}
