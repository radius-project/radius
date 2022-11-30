// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// MetricsProviderOptions represents the options of the providers for publishing metrics.
type MetricsProviderOptions struct {
	Prometheus PrometheusOptions `yaml:"prometheus,omitempty"`
}

// PrometheusOptions represents prometheus metrics provider options.
type PrometheusOptions struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
	Port    int    `yaml:"port"`
}
