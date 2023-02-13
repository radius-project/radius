// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package trace

type TracerProviderOptions struct {
	Zipkin *ZipkinOptions `yaml:"zipkin,omitempty"`
}

// ZipkinOptions represents zipkin trace provider options.
type ZipkinOptions struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}
