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

	directors := []DirectorFunc{}
	if options.TrimPlanesPrefix {
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
