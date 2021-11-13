// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/dapr"
	"github.com/Azure/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprstatestorev1alpha3"
	"github.com/Azure/radius/pkg/renderers/gateway"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/Azure/radius/pkg/renderers/rabbitmqv1alpha3"
	"github.com/Azure/radius/pkg/renderers/redisv1alpha3"
	"github.com/Azure/radius/pkg/renderers/volumev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesModel(k8s client.Client) model.ApplicationModel {

	radiusResources := []model.RadiusResourceModel{
		{
			ResourceType: containerv1alpha3.ResourceType,
			Renderer:     &dapr.Renderer{Inner: &containerv1alpha3.Renderer{}},
		},
		{
			ResourceType: daprhttproutev1alpha3.ResourceType,
			Renderer:     &daprhttproutev1alpha3.Renderer{},
		},
		{
			ResourceType: daprstatestorev1alpha3.ResourceType,
			Renderer:     &daprstatestorev1alpha3.Renderer{StateStores: daprstatestorev1alpha3.SupportedKubernetesStateStoreKindValues},
		},
		{
			ResourceType: mongodbv1alpha3.ResourceType,
			Renderer:     &mongodbv1alpha3.KubernetesRenderer{},
		},
		{
			ResourceType: rabbitmqv1alpha3.ResourceType,
			Renderer:     &rabbitmqv1alpha3.Renderer{},
		},
		{
			ResourceType: redisv1alpha3.ResourceType,
			Renderer:     &redisv1alpha3.KubernetesRenderer{},
		},
		{
			ResourceType: httproutev1alpha3.ResourceType,
			Renderer:     &httproutev1alpha3.Renderer{},
		},
		{
			ResourceType: gateway.ResourceType,
			Renderer:     &gateway.Renderer{},
		},
		{
			ResourceType: volumev1alpha3.ResourceType,
			Renderer:     &volumev1alpha3.KubernetesRenderer{VolumeRenderers: nil},
		},
	}
	outputResources := []model.OutputResourceModel{
		{
			Kind:            resourcekinds.Kubernetes,
			ResourceHandler: handlers.NewKubernetesHandler(k8s),
		},
	}

	return model.NewModel(radiusResources, outputResources)
}
