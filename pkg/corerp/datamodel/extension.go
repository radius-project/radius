/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package datamodel

// ExtensionKind
type ExtensionKind string

const (
	ManualScaling                ExtensionKind = "manualScaling"
	DaprSidecar                  ExtensionKind = "daprSidecar"
	KubernetesMetadata           ExtensionKind = "kubernetesMetadata"
	KubernetesNamespaceExtension ExtensionKind = "kubernetesNamespace"
)

// Extension of a resource.
type Extension struct {
	Kind                ExtensionKind           `json:"kind,omitempty"`
	ManualScaling       *ManualScalingExtension `json:"manualScaling,omitempty"`
	DaprSidecar         *DaprSidecarExtension   `json:"daprSidecar,omitempty"`
	KubernetesMetadata  *KubeMetadataExtension  `json:"kubernetesMetadata,omitempty"`
	KubernetesNamespace *KubeNamespaceExtension `json:"kubernetesNamespace,omitempty"`
}

// KubeMetadataExtension represents the extension of kubernetes resource.
type KubeMetadataExtension struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// KubeNamespaceOverrideExtension represents the extension to override kubernetes namespace.
type KubeNamespaceExtension struct {
	Namespace string `json:"namespace,omitempty"`
}

// FindExtension finds the extension.
func FindExtension(exts []Extension, kind ExtensionKind) *Extension {
	for _, ext := range exts {
		if ext.Kind == kind {
			return &ext
		}
	}
	return nil
}
