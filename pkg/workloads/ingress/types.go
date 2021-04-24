// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ingress

const Kind = "radius.dev/Ingress@v1alpha1"

type Trait struct {
	Kind       string          `json:"kind"`
	Properties TraitProperties `json:"properties"`
}

type TraitProperties struct {
	Hostname string `json:"hostname"`
	Service  string `json:"service"`
}
