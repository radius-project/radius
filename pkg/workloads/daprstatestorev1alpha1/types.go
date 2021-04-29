// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

const Kind = "dapr.io/StateStore@v1alpha1"

// DaprStateStoreComponent is the definition of the container component
type DaprStateStoreComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    DaprStateStoreConfig     `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// DaprStateStoreConfig is the defintion of the config section
type DaprStateStoreConfig struct {
	Kind    string `json:"kind"`
	Managed bool   `json:"managed"`
}
