// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// hostoptions defines and reads options for the metrics's execution environment.

package hostoptions

import (
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
)

// HostOptions defines all of the settings that our metric's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap metrics configuration loaded from config file.
	Config *provider.MetricsProviderOptions
}

// NewHostOptionsFromEnvironment of metrics/hostoptions package returns the HostOptions for metrics service.
func NewHostOptionsFromEnvironment(options hostoptions.ProviderConfig) HostOptions {
	return HostOptions{
		Config: &options.MetricsProvider,
	}
}
