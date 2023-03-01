// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package trace

// Options represents the trace options.
type Options struct {
	// ServiceName represents the name of service.
	ServiceName string `yaml:"serviceName,omitempty"`
	// Zipkin represents zipkin options.
	Zipkin *ZipkinOptions `yaml:"zipkin,omitempty"`
}

// ZipkinOptions represents zipkin trace provider options.
type ZipkinOptions struct {
	// URL represents the url of zipkin endpoint.
	URL string `yaml:"url"`
}
