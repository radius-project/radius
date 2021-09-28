// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscalev1alpha3

const Kind = "radius.dev/ManualScaling@v1alpha1"

type Trait struct {
	Kind     string `json:"kind"`
	Replicas *int32 `json:"replicas,omitempty"`
}
