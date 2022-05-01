// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsprovider

// Represents the data metrics provider options.
type MetricsClientProviderOptions struct {
	MetricsClientProviderOptions PrometheusClientProviderInfo `yaml:"prometheus,omitempty"`
}

// Represents prometheus options for metrics client provider.
type PrometheusClientProviderInfo struct {
	Endpoint    string `yaml:"endpoint"`
}
