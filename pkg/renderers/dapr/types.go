// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

const Kind = "dapr.io/Sidecar@v1alpha1"
const BindingKind = "dapr.io/DaprHttpRoute"

type Trait struct {
	Kind     string `json:"kind"`
	AppID    string `json:"appId"`
	AppPort  int    `json:"appPort"`
	Config   string `json:"config"`
	Protocol string `json:"protocol"`
	Provides string `json:"provides,omitempty"`
}
