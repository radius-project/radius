// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha1

const (
	Kind = "radius.dev/Volume@v1alpha1"
)

// VolumeComponent is the definition of the volume component
type VolumeComponent struct {
	Name   string       `json:"name"`
	Kind   string       `json:"kind"`
	Config VolumeConfig `json:"config"`
}

// VolumeConfig defintion of the config section
type VolumeConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
	Kind     string `json:"kind"` // Volume Kind
}

// Supported volume kinds
const (
	VolumeKindAzureFileShare = "azure.com.fileshare"
	VolumeKindConfigMap      = "kubernetes.io.ConfigMap"
	VolumeKindEmptyDir       = "kubernetes.io.EmptyDir"
)
