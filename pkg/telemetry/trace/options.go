// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package trace

type Options struct {
	ServiceName string         `yaml:"serviceName,omitempty"`
	Zipkin      *ZipkinOptions `yaml:"zipkin,omitempty"`
}

// ZipkinOptions represents zipkin trace provider options.
type ZipkinOptions struct {
	URL string `yaml:"url"`
}
