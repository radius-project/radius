// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/model"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprstatestorev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/gateway"
	"github.com/project-radius/radius/pkg/renderers/httproutev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/microsoftsqlv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/rabbitmqv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/redisv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/volumev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
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
			Renderer:     &rabbitmqv1alpha3.KubernetesRenderer{},
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
		{
			ResourceType: microsoftsqlv1alpha3.ResourceType,
			Renderer:     &microsoftsqlv1alpha3.Renderer{},
		},
		{
			ResourceType: daprpubsubv1alpha3.ResourceType,
			Renderer: &daprpubsubv1alpha3.Renderer{
				PubSubs: daprpubsubv1alpha3.SupportedKubernetesPubSubKindValues,
			},
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
