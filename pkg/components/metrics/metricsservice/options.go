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

package metricsservice

// Options represents the options of the providers for publishing metrics.
type Options struct {
	// Enabled is the flag to enable the metrics service.
	Enabled bool `yaml:"enabled"`

	// ServiceName is the name of the service.
	ServiceName string `yaml:"serviceName,omitempty"`

	// Prometheus is the options for the prometheus metrics provider.
	Prometheus *PrometheusOptions `yaml:"prometheus,omitempty"`
}

// PrometheusOptions represents prometheus metrics provider options.
type PrometheusOptions struct {
	// Path is the path where the prometheus metrics are exposed.
	Path string `yaml:"path"`

	// Address is the address where the prometheus metrics are exposed.
	Port int `yaml:"port"`
}
