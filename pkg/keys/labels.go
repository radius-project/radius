// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keys

// Commonly-used and Radius-Specific labels for Kubernetes
const (
	LabelRadiusApplication = "radius.dev/application"
	LabelRadiusComponent   = "radius.dev/component"
	LabelRadiusRevision    = "radius.dev/revision"

	LabelKubernetesPartOf            = "app.kubernetes.io/part-of"
	LabelKubernetesName              = "app.kubernetes.io/name"
	LabelKubernetesManagedBy         = "app.kubernetes.io/managed-by"
	LabelKubernetesManagedByRadiusRP = "radius-rp"

	FieldManager = "radius-rp"
)
