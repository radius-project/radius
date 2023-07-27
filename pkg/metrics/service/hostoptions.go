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

import (
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/metrics/provider"
)

// HostOptions defines all of the settings that our metric's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap metrics configuration loaded from config file.
	Config *provider.MetricsProviderOptions
}

// # Function Explanation
//
// NewHostOptionsFromEnvironment creates a new HostOptions object from a ProviderConfig object.
func NewHostOptionsFromEnvironment(options hostoptions.ProviderConfig) HostOptions {
	return HostOptions{
		Config: &options.MetricsProvider,
	}
}
