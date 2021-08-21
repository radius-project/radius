// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/inboundroute"
	"github.com/Azure/radius/pkg/workloads/manualscale"
	"github.com/Azure/radius/pkg/workloads/mongodbv1alpha1"
	"github.com/Azure/radius/pkg/workloads/rabbitmqv1alpha1"
	"github.com/Azure/radius/pkg/workloads/redisv1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesModel(k8s *client.Client) model.ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		containerv1alpha1.Kind:      &manualscale.Renderer{Inner: &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: armauth.ArmConfig{}}}}},
		daprstatestorev1alpha1.Kind: &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedKubernetesStateStoreKindValues},
		mongodbv1alpha1.Kind:        &mongodbv1alpha1.KubernetesRenderer{},
		rabbitmqv1alpha1.Kind:       &rabbitmqv1alpha1.Renderer{},
		redisv1alpha1.Kind:          &redisv1alpha1.KubernetesRenderer{},
	}
	handlers := map[string]model.Handlers{
		outputresource.KindKubernetes: {ResourceHandler: handlers.NewKubernetesHandler(*k8s), HealthHandler: nil},
	}
	return model.NewModel(renderers, handlers)
}
