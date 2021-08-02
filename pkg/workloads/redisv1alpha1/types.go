// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

const Kind = "redislabs.com/Redis@v1alpha1"
const BindingKind = "redislabs.com/Redis"

// RedisComponent is the definition of the container component
type RedisComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   RedisConfig              `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// RedisConfig is the defintion of the config section
type RedisConfig struct {
	Managed bool `json:"managed"`
}
