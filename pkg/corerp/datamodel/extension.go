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
	Kind                        ExtensionKind                    `json:"kind,omitempty"`
	ManualScaling               *ManualScalingExtension          `json:"manualScaling,omitempty"`
	DaprSidecar                 *DaprSidecarExtension            `json:"daprSidecar,omitempty"`
	KubernetesMetadata          *BaseKubernetesMetadataExtension `json:"kubernetesMetadata,omitempty"`
	KubernetesNamespaceOverride *BaseK8sNSOverrideExtension      `json:"kubernetesNamespaceOverride,omitempty"`
}

// BaseKubernetesMetadataExtension - Specifies base struct for user defined labels and annotations
type BaseKubernetesMetadataExtension struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type BaseK8sNSOverrideExtension struct {
	Namespace string `json:"namespace,omitempty"`
}
