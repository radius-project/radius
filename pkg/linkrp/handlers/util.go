// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const daprConflictFmt = "the Dapr component name '%q' is already in use by another resource. Dapr component and resource names must be unique across all Dapr types (eg: StateStores, PubSubBrokers, SecretStores, etc.). Please select a new name and try again"

func convertToUnstructured(resource rpv1.OutputResource) (unstructured.Unstructured, error) {
	if resource.ResourceType.Provider != resourcemodel.ProviderKubernetes {
		return unstructured.Unstructured{}, errors.New("wrong resource type")
	}

	obj, ok := resource.Resource.(runtime.Object)
	if !ok {
		return unstructured.Unstructured{}, errors.New("inner type was not a runtime.Object")
	}

	c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource.Resource)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("could not convert object %v to unstructured: %w", obj.GetObjectKind(), err)
	}

	return unstructured.Unstructured{Object: c}, nil
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

	return fmt.Errorf(daprConflictFmt, componentName)
}
