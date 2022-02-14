// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package v1alpha3 contains API Schema definitions for the radius runtime v1alpha3 API group
//+kubebuilder:object:generate=true
//+groupName=runtime.radius.dev
package v1alpha3

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "runtime.radius.dev", Version: "v1alpha3"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
