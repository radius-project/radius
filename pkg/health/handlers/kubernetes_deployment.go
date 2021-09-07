// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/health/handleroptions"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcekinds"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const KubernetesLabelName = "app.kubernetes.io/name"

func NewKubernetesDeploymentHandler(k8s kubernetes.Interface) HealthHandler {
	return &kubernetesDeploymentHandler{k8s: k8s}
}

type kubernetesDeploymentHandler struct {
	k8s kubernetes.Interface
}

func (handler *kubernetesDeploymentHandler) GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options handleroptions.Options) healthcontract.ResourceHealthDataMessage {
	kID, err := healthcontract.ParseK8sResourceID(resourceInfo.ResourceID)
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	logger := radlogger.GetLogger(ctx)

	// Start watching deployment changes
	w, err := handler.k8s.CoreV1().Pods(kID.Namespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		LabelSelector: fmt.Sprintf("%s=%s", KubernetesLabelName, kID.Name),
	})
	if err != nil {
		msg := healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
		options.WatchHealthChangesChannel <- msg
		return msg
	}
	defer w.Stop()

	podsChan := w.ResultChan()
	for {
		select {
		case podEvent := <-podsChan:
			healthState := ""
			healthStateErrorDetails := ""
			pod, ok := podEvent.Object.(*corev1.Pod)
			if !ok {
				healthState = healthcontract.HealthStateUnhealthy
				healthStateErrorDetails = "Object is not a pod"
			} else {
				if pod.Status.Phase == corev1.PodRunning {
					healthState = healthcontract.HealthStateHealthy
					healthStateErrorDetails = ""
				} else {
					healthState = healthcontract.HealthStateUnhealthy
					healthStateErrorDetails = pod.Status.Reason
				}
			}

			// Notify the watcher. Let the watcher determine if an action is needed
			msg := healthcontract.ResourceHealthDataMessage{
				Resource: healthcontract.ResourceInfo{
					HealthID:     resourceInfo.HealthID,
					ResourceID:   resourceInfo.ResourceID,
					ResourceKind: resourcekinds.KindKubernetes,
				},
				HealthState:             healthState,
				HealthStateErrorDetails: healthStateErrorDetails,
			}
			options.WatchHealthChangesChannel <- msg
			logger.Info(fmt.Sprintf("Detected health change event for Resource: %s. Notifying watcher.", resourceInfo.ResourceID))
		case <-options.StopChannel:
			logger.Info(fmt.Sprintf("Stopped health monitoring for namespace: %v", kID.Namespace))
			return healthcontract.ResourceHealthDataMessage{}
		}
	}
}
