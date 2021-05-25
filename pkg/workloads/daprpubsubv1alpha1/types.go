// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

const Kind = "dapr.io/PubSubTopic@v1alpha1"

// DaprPubSubComponent is the definition of the container component
type DaprPubSubComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    DaprPubSubConfig         `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// DaprPubSubConfig is the defintion of the config section
type DaprPubSubConfig struct {
	Kind     string `json:"kind"`
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
	Name     string `json:"name"`
	Topic    string `json:"topic"`
}
