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

// NewARMProxy creates a ReverseProxy with custom directors, transport and responders to process requests and responses.
func NewARMProxy(options ReverseProxyOptions, downstream *url.URL, configure func(builder *ReverseProxyBuilder)) ReverseProxy {
	builder := ReverseProxyBuilder{
		Downstream:    downstream,
		EnableLogging: true,
		Transport:     options.RoundTripper,
		Responders:    []ResponderFunc{ProcessAsyncOperationHeaders},
	}

	if configure != nil {
		configure(&builder)
	}

	return builder.Build()
}
