// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

const Kind = "radius.dev/Redis@v1alpha1"

// RedisComponent is the definition of the container component
type RedisComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    RedisConfig              `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// RedisConfig is the defintion of the config section
type RedisConfig struct {
	Kind    string `json:"kind"`
	Managed bool   `json:"managed"`
}
