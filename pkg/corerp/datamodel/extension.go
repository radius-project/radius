// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

// ExtensionKind
type ExtensionKind string

const (
	ManualScaling               ExtensionKind = "manualScaling"
	DaprSidecar                 ExtensionKind = "daprSidecar"
	KubernetesMetadata          ExtensionKind = "kubernetesMetadata"
	KubernetesNamespaceOverride ExtensionKind = "kubernetesNamespaceOverride"
)

// Extension of a resource.
type Extension struct {
	Kind                        ExtensionKind                   `json:"kind,omitempty"`
	ManualScaling               *ManualScalingExtension         `json:"manualScaling,omitempty"`
	DaprSidecar                 *DaprSidecarExtension           `json:"daprSidecar,omitempty"`
	KubernetesMetadata          *KubeMetadataExtension          `json:"kubernetesMetadata,omitempty"`
	KubernetesNamespaceOverride *KubeNamespaceOverrideExtension `json:"kubernetesNamespaceOverride,omitempty"`
}

// KubeMetadataExtension represents the extension of kubernetes resource.
type KubeMetadataExtension struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// KubeNamespaceOverrideExtension represents the extension to override kubernetes namespace.
type KubeNamespaceOverrideExtension struct {
	Namespace string `json:"namespace,omitempty"`
}
