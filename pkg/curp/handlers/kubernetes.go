// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesHandler(k8s client.Client) ResourceHandler {
	return &kubernetesHandler{k8s: k8s}
}

type kubernetesHandler struct {
	k8s client.Client
}

func (handler *kubernetesHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	item, err := convertToUnstructured(options.Resource)
	if err != nil {
		return nil, err
	}

	// For a Kubernetes resource we only need to store the ObjectMeta and TypeMeta data
	p := map[string]string{
		KubernetesKindKey:       item.GetKind(),
		KubernetesAPIVersionKey: item.GetAPIVersion(),
		KubernetesNamespaceKey:  item.GetNamespace(),
		ComponentNameKey:        item.GetName(),
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return p, err
}

func (handler *kubernetesHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ComponentNameKey],
			},
		},
	}

	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}

func convertToUnstructured(resource workloads.WorkloadResource) (unstructured.Unstructured, error) {
	if resource.Type != workloads.ResourceKindKubernetes {
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
