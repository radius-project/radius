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
	"github.com/project-radius/radius/pkg/rp/outputresource"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func convertToUnstructured(resource outputresource.OutputResource) (unstructured.Unstructured, error) {
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

func checkResourceNameUniqueness(ctx context.Context, k8s client.Client, resourceName string, namespace string, resourceType string) error {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Component",
		Version: "dapr.io/v1alpha1",
	})
	err := k8s.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      resourceName,
	}, u)
	// Object with the same name doesn't exist.
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// Object with the same name exists, checking the labels to see if they are the same objects.
	if label, ok := u.GetLabels()[kubernetes.LabelRadiusResourceType]; ok && kubernetes.ConvertLabelToResourceType(label) != strings.ToLower(resourceType) {
		return fmt.Errorf("the Dapr component name '%q' is already in use by another resource. Dapr component and resource names must be unique across all Dapr types (eg: StateStores, PubSubBrokers, SecretStores, etc.). Please select a new name and try again", resourceName)
	}

	return nil
}
