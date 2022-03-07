// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprStateStoreGenericHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreGenericHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreGenericHandler struct {
	kubernetesHandler
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreGenericHandler) patchDaprStateStore(ctx context.Context, options *PutOptions, properties map[string]string) (unstructured.Unstructured, error) {
	err := handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	item, err := constructDaprGeneric(properties, options.ApplicationName, options.ResourceName)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	return item, nil
}

func (handler *daprStateStoreGenericHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	item, err := handler.patchDaprStateStore(ctx, options, properties)
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.ResourceIdentity{
		Kind: resourcemodel.IdentityKindKubernetes,
		Data: resourcemodel.KubernetesIdentity{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Kind:       item.GetKind(),
			APIVersion: item.GetAPIVersion(),
		},
	}

	return properties, nil
}

func (handler *daprStateStoreGenericHandler) Delete(ctx context.Context, options DeleteOptions) error {
	item := getDaprGenericForDelete(ctx, options)

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	return nil
}

func NewDaprStateStoreGenericHealthHandler(arm armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprStateStoreGenericHealthHandler{
		arm: arm,
		k8s: k8s,
	}
}

type daprStateStoreGenericHealthHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreGenericHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
