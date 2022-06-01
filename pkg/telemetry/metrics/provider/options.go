// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// MetricsProviderOptions represents the info of the providers for publishing metrics.
type MetricsProviderOptions struct {
	Prometheus PrometheusOptions `yaml:"prometheus,omitempty"`
}

// PrometheusOptions represents prometheus metrics provider info.
type PrometheusOptions struct {
	Port int `yaml:"port"`
	Endpoint string `yaml:"endpoint"`
}

