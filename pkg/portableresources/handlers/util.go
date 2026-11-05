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

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/portableresources/processors"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const daprConflictFmt = "the Dapr component name '%q' is already in use by another resource. Dapr component and resource names must be unique across all Dapr types (e.g., StateStores, PubSubBrokers, SecretStores, ConfigurationStores, etc.). Please select a new name and try again."

// ApplyDaprComponent creates a component atomically, or updates it when the same Radius resource owns it.
func ApplyDaprComponent(ctx context.Context, k8s client.Client, component *unstructured.Unstructured, resourceName string, resourceType string) error {
	// A read followed by server-side apply lets concurrent resources both claim an unused name.
	err := k8s.Create(ctx, component, client.FieldOwner(kubernetes.FieldManager))
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return &processors.ResourceError{Inner: err}
	}

	existing := component.DeepCopy()
	err = k8s.Get(ctx, client.ObjectKeyFromObject(component), existing)
	if err != nil {
		return &processors.ResourceError{Inner: err}
	}
	err = checkDaprComponentOwner(existing, resourceName, resourceType)
	if err != nil {
		return &processors.ValidationError{Message: err.Error()}
	}

	// Fence the ownership check against deletion or replacement before the apply.
	component.SetResourceVersion(existing.GetResourceVersion())
	//nolint:staticcheck // Unstructured Dapr components do not have generated ApplyConfiguration types.
	err = k8s.Patch(ctx, component, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return &processors.ResourceError{Inner: err}
	}
	return nil
}

// CheckDaprResourceNameUniqueness checks if the resource name is unique in the namespace. If the resource name is not unique, it returns an error.
//
// This protects against some of the following scenarios:
//
// - Two Dapr resources with the same component name but different types.
// - Two Dapr resources with different UCP resource names, but the same Dapr component name.
//
// Note: the Dapr component name and UCP resource name are NOT the same thing. Users can override the Dapr component name.
func CheckDaprResourceNameUniqueness(ctx context.Context, k8s client.Client, componentName string, namespace string, resourceName string, resourceType string) error {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Component",
		Version: "dapr.io/v1alpha1",
	})
	err := k8s.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      componentName,
	}, u)
	if k8serrors.IsNotFound(err) {
		// Object with the same name doesn't exist.
		return nil
	} else if err != nil {
		return err
	}

	return checkDaprComponentOwner(u, resourceName, resourceType)
}

func checkDaprComponentOwner(u *unstructured.Unstructured, resourceName string, resourceType string) error {
	// Object with the same name exists, checking the labels to see if they are 'owned' by the same Radius
	// resource.
	//
	// We also need to handle the case where the component has no Radius labels.
	resourceTypeLabel := u.GetLabels()[kubernetes.LabelRadiusResourceType]
	resourceNameLabel := u.GetLabels()[kubernetes.LabelRadiusResource]
	if strings.EqualFold(resourceNameLabel, resourceName) &&
		strings.EqualFold(kubernetes.ConvertLabelToResourceType(resourceTypeLabel), resourceType) {
		return nil
	}

	return fmt.Errorf(daprConflictFmt, u.GetName())
}
