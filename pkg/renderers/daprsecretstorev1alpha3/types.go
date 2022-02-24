// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

const (
	ResourceType = "dapr.io.SecretStore"
)

type Properties struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}
