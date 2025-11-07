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
func MakeDescriptiveLabels(application string, resource string, resourceType string) (map[string]string, error) {
	normalizedApp, err := NormalizeResourceName(application)
	if err != nil {
		return nil, fmt.Errorf("invalid application name: %w", err)
	}
	
	normalizedResource, err := NormalizeResourceName(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid resource name: %w", err)
	}
	
	return map[string]string{
		LabelRadiusApplication:  normalizedApp,
		LabelRadiusResource:     normalizedResource,
		LabelRadiusResourceType: strings.ToLower(ConvertResourceTypeToLabelValue(resourceType)),
		LabelName:               normalizedResource,
		LabelPartOf:             normalizedApp,
		LabelManagedBy:          LabelManagedByRadiusRP,
	}, nil
}

// MakeDescriptiveDaprLabels returns a map of the descriptive labels for a Kubernetes Dapr resource associated with a Radius resource.
// The descriptive labels are a superset of the selector labels.
func MakeDescriptiveDaprLabels(application string, resource string, resourceType string) (map[string]any, error) {
	// K8s fake client requires this to be map[string]any :(
	//
	// Please don't try to change this to map[string]string as it is going to cause some tests to panic
	// with an error deep inside Kubernetes code.
	normalizedApp, err := NormalizeResourceName(application)
	if err != nil {
		return nil, fmt.Errorf("invalid application name: %w", err)
	}
	
	normalizedResource, err := NormalizeDaprResourceName(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid resource name: %w", err)
	}
	
	return map[string]any{
		LabelRadiusApplication:  normalizedApp,
		LabelRadiusResource:     normalizedResource,
		LabelRadiusResourceType: strings.ToLower(ConvertResourceTypeToLabelValue(resourceType)),
		LabelName:               normalizedResource,
		LabelPartOf:             normalizedApp,
		LabelManagedBy:          LabelManagedByRadiusRP,
	}, nil
}

// MakeSelectorLabels returns a map of labels suitable for a Kubernetes selector to identify a labeled Radius-managed
// Kubernetes object.
//
// This function is used to generate the labels used by a Deployment to select its Pods. eg: the Deployment and Pods
// are the same resource.
func MakeSelectorLabels(application string, resource string) (map[string]string, error) {
	if resource != "" {
		normalizedApp, err := NormalizeResourceName(application)
		if err != nil {
			return nil, fmt.Errorf("invalid application name: %w", err)
		}
		
		normalizedResource, err := NormalizeResourceName(resource)
		if err != nil {
			return nil, fmt.Errorf("invalid resource name: %w", err)
		}
		
		return map[string]string{
			LabelRadiusApplication: normalizedApp,
			LabelRadiusResource:    normalizedResource,
		}, nil
	}
	return map[string]string{
		LabelRadiusApplication: application,
	}, nil
}

// NormalizeResourceName normalizes resource name used for kubernetes resource name scoped in namespace.
// All name will be validated by swagger validation so that it does not get non-RFC1035 compliant characters.
// Therefore, this function will lowercase the name without allowed character validation.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#rfc-1035-label-names
func NormalizeResourceName(name string) (string, error) {
	normalized := strings.ToLower(name)
	if normalized == "" {
		return normalized, nil
	}

	if !IsValidObjectName(normalized) {
		return "", fmt.Errorf("invalid Kubernetes resource name: %q does not comply with RFC 1035 (DNS label) requirements. Resource names must consist of lowercase alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character", name)
	}
	return normalized, nil
}

// NormalizeDaprResourceName normalizes resource name used for kubernetes Dapr resource name scoped in namespace.
// All name will be validated by swagger validation so that it does not get non-RFC1035 compliant characters.
// Therefore, this function will lowercase the name without allowed character validation. This function returns
// an error if the name is invalid.
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#rfc-1035-label-names
func NormalizeDaprResourceName(name string) (string, error) {
	normalized := strings.ToLower(name)
	if normalized == "" {
		return normalized, nil
	}

	if !IsValidDaprObjectName(normalized) {
		return "", fmt.Errorf("invalid Kubernetes Dapr resource name: %q does not comply with RFC 1123 (DNS subdomain) requirements. Resource names must consist of lowercase alphanumeric characters, '-' or '.', start and end with an alphanumeric character", name)
	}
	return normalized, nil
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
