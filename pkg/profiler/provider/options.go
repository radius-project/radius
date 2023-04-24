// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// ProfilerProviderOptions represents the options for enabling pprof profiler.
type ProfilerProviderOptions struct {
	Enabled bool `yaml:"enabled,omitempty"`
	Port    int  `yaml:"port,omitempty"`
}
