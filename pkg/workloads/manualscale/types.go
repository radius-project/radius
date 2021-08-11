// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscale

const Kind = "radius.dev/ManualScaling@v1alpha1"

type Trait struct {
	Kind     string `json:"kind"`
	Replicas *int32 `json:"replicas"` // TODO figure out if pointer here makes sense to distinguish between 0 and not set.
}
