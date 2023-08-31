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

package profilerservice

import (
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/profiler/provider"
)

type HostOptions struct {
	// Config is the bootstrap profiler configuration loaded from config file.
	Config *provider.ProfilerProviderOptions
}

// NewHostOptionsFromEnvironment of profiler/hostoptions package returns the HostOptions for profiler service.
func NewHostOptionsFromEnvironment(options hostoptions.ProviderConfig) HostOptions {
	return HostOptions{
		Config: &options.ProfilerProvider,
	}
}
