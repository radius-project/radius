// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
	"net/http"
	"net/url"
)

type Options struct {
}
type armProxy struct {
	ProxyAddress string
}

// NewARMProxy creates a proxy that understands ARM's protocol
func NewARMProxy(options ReverseProxyOptions, downstream *url.URL, configure func(builder *ReverseProxyBuilder)) ReverseProxy {
	p := armProxy{
		ProxyAddress: options.ProxyAddress,
	}

	directors := []func(r *http.Request){}
	if !options.UCPNativeProxy {
		// Remove the UCP Planes prefix for non-native planes that do not
		// understand UCP IDs
		directors = []DirectorFunc{trimPlanesPrefix}
	}

	builder := ReverseProxyBuilder{
		Downstream:    downstream,
		EnableLogging: true,
		Directors:     directors,
		Transport: Transport{
			roundTripper: options.RoundTripper,
		},
		Responders: []ResponderFunc{p.processAsyncResponse},
	}

	if configure != nil {
		configure(&builder)
	}

	return builder.Build()
}
