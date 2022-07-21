// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	osm "github.com/openservicemesh/osm/pkg/constants"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/store"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KubernetesAPIVersionKey = "kubernetesapiversion"
	KubernetesKindKey       = "kuberneteskind"
	KubernetesNamespaceKey  = "kubernetesnamespace"
	KubernetesNameKey       = "kubernetesname"
	ResourceName            = "resourcename"
	ApplicationName         = "applicationName"
)

func NewKubernetesHandler(k8s client.Client) ResourceHandler {
	return &kubernetesHandler{k8s: k8s}
}

type kubernetesHandler struct {
	k8s client.Client
}

func (handler *kubernetesHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (resourcemodel.ResourceIdentity, map[string]string, error) {
	item, err := convertToUnstructured(*resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = handler.PatchNamespace(ctx, item.GetNamespace())
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// For a Kubernetes resource we only need to store the ObjectMeta and TypeMeta data
	properties := map[string]string{
		KubernetesKindKey:       item.GetKind(),
		KubernetesAPIVersionKey: item.GetAPIVersion(),
		KubernetesNamespaceKey:  item.GetNamespace(),
		ResourceName:            item.GetName(),
	}

	if resource.Deployed {
		// This resource is deployed in the Render process
		// TODO: This will eventually change
		// For now, no need to process any further
		return resourcemodel.ResourceIdentity{}, properties, nil
	}
	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	resource.Identity = resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resource.ResourceType.Type,
			Provider: providers.ProviderKubernetes,
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

func (handler *kubernetesHandler) PatchNamespace(ctx context.Context, namespace string) error {
	// Ensure that the namespace exists that we're able to operate upon.
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespace,
				"labels": map[string]interface{}{
					kubernetes.LabelManagedBy:            kubernetes.LabelManagedByRadiusRP,
					osm.OSMKubeResourceMonitorAnnotation: "osm",
				},
				"annotations": map[string]interface{}{
					osm.SidecarInjectionAnnotation: "enabled",
				},
			},
		},
	}

	err := handler.k8s.Patch(ctx, ns, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		// we consider this fatal - without a namespace we won't be able to apply anything else
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}

func (handler *kubernetesHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	identity := &resourcemodel.KubernetesIdentity{}
	if err := store.DecodeMap(resource.Identity.Data, identity); err != nil {
		return err
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": identity.APIVersion,
			"kind":       identity.Kind,
			"metadata": map[string]interface{}{
				"namespace": identity.Namespace,
				"name":      identity.Name,
			},
		},
	}

	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}

func convertToUnstructured(resource outputresource.OutputResource) (unstructured.Unstructured, error) {
	if resource.ResourceType.Provider != providers.ProviderKubernetes {
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
