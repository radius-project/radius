// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	radiusv1alpha3 "github.com/project-radius/radius/pkg/kubernetes/api/radius/v1alpha3"
)

const (
	CacheKeySpecApplication = "metadata.application"
	CacheKeyController      = "metadata.controller"
)

func extractOwnerKey(obj client.Object) []string {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return nil
	}

	// Assume all other types besides Application are owned by us with radius.
	if owner.APIVersion != radiusv1alpha3.GroupVersion.String() || owner.Kind == "Application" {
		return nil
	}

	return []string{owner.Kind + owner.Name}
}
