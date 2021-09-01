// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/healthcontract"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
)

func NewKubernetesServiceHandler(k8s k8s.Clientset) HealthHandler {
	return &kubernetesServiceHandler{k8s: k8s}
}

type kubernetesServiceHandler struct {
	k8s k8s.Clientset
}

func (handler *kubernetesServiceHandler) GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options Options) healthcontract.ResourceHealthDataMessage {
	kID, err := healthcontract.ParseK8sResourceID(resourceInfo.ResourceID)
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	// Only checking service existence as status
	_, err = handler.k8s.CoreV1().Services(kID.Namespace).Get(ctx, kID.Name, metav1.GetOptions{})
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	return healthcontract.ResourceHealthDataMessage{
		Resource:                resourceInfo,
		HealthState:             healthcontract.HealthStateHealthy,
		HealthStateErrorDetails: "",
	}
}
