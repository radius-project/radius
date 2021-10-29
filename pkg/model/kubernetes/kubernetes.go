// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/Azure/radius/pkg/handlers"
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/dapr"
	"github.com/Azure/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/renderers/gateway"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/Azure/radius/pkg/renderers/rabbitmqv1alpha1"
	"github.com/Azure/radius/pkg/renderers/redisv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesModel(k8s *client.Client) model.ApplicationModel {
	renderers := map[string]renderers.Renderer{
		containerv1alpha3.ResourceType:      &dapr.Renderer{Inner: &containerv1alpha3.Renderer{}},
		daprhttproutev1alpha3.ResourceType:  &daprhttproutev1alpha3.Renderer{},
		daprstatestorev1alpha1.ResourceType: &renderers.V1RendererAdapter{Inner: &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedKubernetesStateStoreKindValues}},
		mongodbv1alpha3.ResourceType:        &mongodbv1alpha3.KubernetesRenderer{},
		rabbitmqv1alpha1.ResourceType:       &renderers.V1RendererAdapter{Inner: &rabbitmqv1alpha1.Renderer{}},
		redisv1alpha3.ResourceType:          &redisv1alpha3.KubernetesRenderer{},
		httproutev1alpha3.ResourceType:      &httproutev1alpha3.Renderer{},
		gateway.ResourceType:                &gateway.Renderer{},
	}

	handlers := map[string]model.Handlers{
		resourcekinds.Kubernetes: {ResourceHandler: handlers.NewKubernetesHandler(*k8s), HealthHandler: nil},
	}
	return model.NewModel(renderers, handlers)
}
