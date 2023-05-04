// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprComponentHandler(k8s client.Client) ResourceHandler {
	return &daprComponentHandler{k8s: k8s}
}

type daprComponentHandler struct {
	k8s client.Client
}

func (handler *daprComponentHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	item, err := convertToUnstructured(*resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = kubeutil.PatchNamespace(ctx, handler.k8s, item.GetNamespace())
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	properties := map[string]string{
		KubernetesKindKey:       item.GetKind(),
		KubernetesAPIVersionKey: item.GetAPIVersion(),
		KubernetesNamespaceKey:  item.GetNamespace(),
		ResourceName:            item.GetName(),
	}

	if resource.Deployed {
		return resourcemodel.ResourceIdentity{}, properties, nil
	}

	err = CheckDaprResourceNameUniqueness(ctx, handler.k8s, kubernetes.NormalizeDaprResourceName(properties[ResourceName]), properties[KubernetesNamespaceKey], properties[ResourceName], serviceCtx.ResourceID.Type())
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	resource.Identity = resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resource.ResourceType.Type,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Data: resourcemodel.KubernetesIdentity{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Kind:       item.GetKind(),
			APIVersion: item.GetAPIVersion(),
		},
	}

	return resourcemodel.ResourceIdentity{}, properties, err
}

func (handler *daprComponentHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	identity := &resourcemodel.KubernetesIdentity{}
	if err := store.DecodeMap(resource.Identity.Data, identity); err != nil {
		return err
	}

	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": identity.APIVersion,
			"kind":       identity.Kind,
			"metadata": map[string]any{
				"namespace": identity.Namespace,
				"name":      identity.Name,
			},
		},
	}

	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}
