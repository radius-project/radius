// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

// Represents the info for differenct metrics providers
type MetricsOptions struct {
	MetricsOptions PrometheusClientProviderInfo `yaml:"prometheus,omitempty"`
}

// Represents prometheus provider info
type PrometheusClientProviderInfo struct {
	Endpoint string `yaml:"endpoint"`
}
