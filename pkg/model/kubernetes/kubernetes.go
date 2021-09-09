// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	workloads "github.com/Azure/radius/pkg/workloadsv1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesModel(k8s *client.Client) model.ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		containerv1alpha3.Kind: &containerv1alpha3.Renderer{Arm: armauth.ArmConfig{}},
		httproutev1alpha3.Kind: &httproutev1alpha3.Renderer{},
		// daprstatestorev1alpha1.Kind: &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedKubernetesStateStoreKindValues},
		// mongodbv1alpha1.Kind:        &mongodbv1alpha1.KubernetesRenderer{},
		// rabbitmqv1alpha1.Kind:       &rabbitmqv1alpha1.Renderer{},
		// redisv1alpha1.Kind:          &redisv1alpha1.KubernetesRenderer{},
	}

	handlers := map[string]model.Handlers{
		resourcekinds.Kubernetes: {ResourceHandler: handlers.NewKubernetesHandler(*k8s), HealthHandler: nil},
	}
	return model.NewModel(renderers, handlers)
}
