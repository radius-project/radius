// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroute

const Kind = "radius.dev/InboundRoute@v1alpha1"

type Trait struct {
	Kind     string `json:"kind"`
	Hostname string `json:"hostname"`
	Binding  string `json:"binding"`
}
