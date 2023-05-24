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

	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewKubernetesHandler creates a new ResourceHandler for Kubernetes resources.
func NewKubernetesHandler(client client.Client) ResourceHandler {
	return &kubernetesHandler{client: client}
}

type kubernetesHandler struct {
	client client.Client
}

// Put implements Put for Kubernetes resources.
func (handler *kubernetesHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error) {
	gvk, namespace, name, err := resource.Identity.RequireKubernetes()
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	gvk, err = handler.SelectVersion(gvk)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"namespace": namespace,
				"name":      name,
			},
		},
	}

	err = handler.client.Get(ctx, client.ObjectKeyFromObject(&item), &item)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	resource.Resource = item.Object
	return resource.Identity, map[string]string{}, nil
}

// Delete implementes Delete for Kubernetes resources.
func (handler *kubernetesHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	if !resource.IsRadiusManaged() {
		return nil
	}

	gvk, namespace, name, err := resource.Identity.RequireKubernetes()
	if err != nil {
		return err
	}

	gvk, err = handler.SelectVersion(gvk)
	if err != nil {
		return err
	}

	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"namespace": namespace,
				"name":      name,
			},
		},
	}

	return client.IgnoreNotFound(handler.client.Delete(ctx, &item))
}

// SelectVersion can find the preferred API version for a Kubernetes resource. The resource IDs we get back from
// UCP don't contain a version. In the event that we don't care about the version we're using, this function
// helps us stay uncoupled.
func (handler *kubernetesHandler) SelectVersion(gvk schema.GroupVersionKind) (schema.GroupVersionKind, error) {
	if gvk.Version != resourcemodel.APIVersionUnknown {
		return gvk, nil
	}

	mapping, err := handler.client.RESTMapper().RESTMapping(gvk.GroupKind())
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("unable to determine version for %s: %w", gvk.GroupKind().String(), err)
	}

	return mapping.GroupVersionKind, nil
}
