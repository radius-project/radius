// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/inboundroute"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesModel(k8s *client.Client) ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		containerv1alpha1.Kind:      &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: armauth.ArmConfig{}}}},
		daprstatestorev1alpha1.Kind: &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedKubernetesStateStoreKindValues},
	}
	handlers := map[string]handlers.ResourceHandler{
		outputresource.KindKubernetes: handlers.NewKubernetesHandler(*k8s),
	}
	return NewModel(renderers, handlers)
}
