/*
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
*/

package kubernetes

import (
	"fmt"
	"strings"
)

// Commonly-used and Radius-Specific labels for Kubernetes
const (
	LabelRadiusApplication  = "radapp.io/application"
	LabelRadiusResource     = "radapp.io/resource"
	LabelRadiusDeployment   = "radapp.io/deployment"
	LabelRadiusRouteFmt     = "radapp.io/route-%s-%s"
	LabelRadiusResourceType = "radapp.io/resource-type"
	LabelPartOf             = "app.kubernetes.io/part-of"
	LabelName               = "app.kubernetes.io/name"
	LabelManagedBy          = "app.kubernetes.io/managed-by"

	LabelManagedByRadiusRP = "radius-rp"

	FieldManager = "radius-rp"

	// ControlPlanePartOfLabelValue is the value we use for 'app.kubernetes.io/part-of' in Radius's control-plane components.
	// This value can be used to query all of the pods that make up the control plane (for example).
	ControlPlanePartOfLabelValue = "radius"

	AnnotationSecretHash = "radapp.io/secret-hash"
	RadiusDevPrefix      = "radapp.io/"

	// AnnotationIdentityType is the annotation for supported identity.
	AnnotationIdentityType = "radapp.io/identity-type"
)

// NOTE: the difference between descriptive labels and selector labels
//
// descriptive labels:
// - intended for humans and human troubleshooting
// - we include both our own metadata and kubernetes *recommended* labels
// - we might (in the future) include non-deterministic details because these are informational
//
// selector labels:
// - intended for programmatic matching (selecting a pod for logging)
// - no value in redundancy between our own metadata and *recommended* labels, simpler is better
// - MUST remain deterministic
// - MUST be a subset of descriptive labels
//
// In general, descriptive labels should be applied all to Kubernetes objects, and selector labels should be used
// when programmatically querying those objects.

// MakeDescriptiveLabels returns a map of the descriptive labels for a Kubernetes resource associated with a Radius resource.
// The descriptive labels are a superset of the selector labels.
func MakeDescriptiveLabels(application string, resource string, resourceType string) map[string]string {
	return map[string]string{
		LabelRadiusApplication:  NormalizeResourceName(application),
		LabelRadiusResource:     NormalizeResourceName(resource),
		LabelRadiusResourceType: strings.ToLower(ConvertResourceTypeToLabelValue(resourceType)),
		LabelName:               NormalizeResourceName(resource),
		LabelPartOf:             NormalizeResourceName(application),
		LabelManagedBy:          LabelManagedByRadiusRP,
	}
}

// MakeDescriptiveLabels returns a map of the descriptive labels for a Kubernetes Dapr resource associated with a Radius resource.
// The descriptive labels are a superset of the selector labels.
func MakeDescriptiveDaprLabels(application string, resource string, resourceType string) map[string]any {
	// K8s fake client requires this to be map[string]any :(
	//
	// Please don't try to change this to map[string]string as it is going to cause some tests to panic
	// with an error deep inside Kubernetes code.
	return map[string]any{
		LabelRadiusApplication:  NormalizeResourceName(application),
		LabelRadiusResource:     NormalizeDaprResourceName(resource),
		LabelRadiusResourceType: strings.ToLower(ConvertResourceTypeToLabelValue(resourceType)),
		LabelName:               NormalizeDaprResourceName(resource),
		LabelPartOf:             NormalizeResourceName(application),
		LabelManagedBy:          LabelManagedByRadiusRP,
	}
}

// MakeSelectorLabels returns a map of labels suitable for a Kubernetes selector to identify a labeled Radius-managed
// Kubernetes object.
//
// This function is used to generate the labels used by a Deployment to select its Pods. eg: the Deployment and Pods
// are the same resource.
func MakeSelectorLabels(application string, resource string) map[string]string {
	if resource != "" {
		return map[string]string{
			LabelRadiusApplication: NormalizeResourceName(application),
			LabelRadiusResource:    NormalizeResourceName(resource),
		}
	}
	return map[string]string{
		LabelRadiusApplication: application,
	}
}

// MakeRouteSelectorLabels returns a map of labels suitable for a Kubernetes selector to identify a labeled Radius-managed
// Kubernetes object.
//
// This function differs from MakeSelectorLablels in that it's intended to *cross* resources. eg: The Service created by
// an HttpRoute and the Deployment created by a Container.
func MakeRouteSelectorLabels(application string, resourceType string, route string) map[string]string {
	return map[string]string{
		LabelRadiusApplication: NormalizeResourceName(application),

		// NOTE: pods can serve multiple routes of different types. Therefore we need to encode the
		// the route's type and name in the *key* to support multiple matches.
		fmt.Sprintf(LabelRadiusRouteFmt, NormalizeResourceName(strings.TrimSuffix(resourceType, "Route")), NormalizeResourceName(route)): "true",
	}
}

// NormalizeResourceName normalizes resource name used for kubernetes resource name scoped in namespace.
// All name will be validated by swagger validation so that it does not get non-RFC1035 compliant characters.
// Therefore, this function will lowercase the name without allowed character validation.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#rfc-1035-label-names
func NormalizeResourceName(name string) string {
	normalized := strings.ToLower(name)
	if normalized == "" {
		return normalized
	}

	if !IsValidObjectName(normalized) {
		// This should not happen.
		panic(normalized + " is an invalid name.")
	}
	return normalized
}

// NormalizeDaprResourceName normalizes resource name used for kubernetes Dapr resource name scoped in namespace.
// All name will be validated by swagger validation so that it does not get non-RFC1035 compliant characters.
// Therefore, this function will lowercase the name without allowed character validation. This function panics
// if the name is invalid.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#rfc-1035-label-names
func NormalizeDaprResourceName(name string) string {
	normalized := strings.ToLower(name)
	if normalized == "" {
		return normalized
	}

	if !IsValidDaprObjectName(normalized) {
		// This should not happen.
		panic(normalized + " is an invalid name.")
	}
	return normalized
}

// ConvertResourceTypeToLabelValue converts the given string to a value that Kubernetes allows i.e.
// it replaces the first occurrence of "/" with "-" and returns the modified string.
// Example: Applications.Core/containers becomes Applications.Core-Containers
func ConvertResourceTypeToLabelValue(resourceType string) string {
	return strings.Replace(resourceType, "/", "-", 1)
}

// ConvertLabelToResourceType converts from kubernetes label value to Radius resource type i.e.
// it replaces the first occurrence of "-" with "/" in the given string and returns the result.
// Example: Applications.Core-containers becomes Applications.Core/Containers
func ConvertLabelToResourceType(labelValue string) string {
	return strings.Replace(labelValue, "-", "/", 1)
}
