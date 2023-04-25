// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package profilerservice

import (
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/profiler/provider"
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
