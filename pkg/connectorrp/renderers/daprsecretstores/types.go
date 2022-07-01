// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

const (
	ResourceType = "Applications.Connector/daprSecretStores"
)

type Properties struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}
