// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import (
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
	builder := ReverseProxyBuilder{
		Downstream:    downstream,
		EnableLogging: true,
		Directors:     []DirectorFunc{trimPlanesPrefix},
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
