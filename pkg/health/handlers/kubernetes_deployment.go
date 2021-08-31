// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func NewKubernetesDeploymentHandler(k8s client.Client) HealthHandler {
	return &kubernetesDeploymentHandler{k8s: k8s}
}

type kubernetesDeploymentHandler struct {
	k8s client.Client
}

func (handler *kubernetesDeploymentHandler) GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options healthcontract.HealthCheckOptions) healthcontract.ResourceHealthDataMessage {
	kID, err := healthcontract.ParseK8sResourceID(resourceInfo.ResourceID)
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	cfg, err := config.GetConfig()
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	// Start watching deployment changes
	notificationCh := ctx.Value("notifyCh").(chan healthcontract.ResourceHealthDataMessage)
	stopCh := ctx.Value("stopCh").(chan struct{})
	logger := radlogger.GetLogger(ctx)

	w, err := k8s.CoreV1().Pods(kID.Namespace).Watch(ctx, metav1.ListOptions{Watch: true})
	if err != nil {
		healthStateErrorDetails := err.Error()
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: healthStateErrorDetails,
		}
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

			// Health state has changed. Notify the watcher
			if healthState != "" {
				msg := healthcontract.ResourceHealthDataMessage{
					Resource: healthcontract.ResourceInfo{
						HealthID:     resourceInfo.HealthID,
						ResourceID:   resourceInfo.ResourceID,
						ResourceKind: "ResourceKindKubernetes",
					},
					HealthState:             healthState,
					HealthStateErrorDetails: healthStateErrorDetails,
				}
				notificationCh <- msg
				logger.Info(fmt.Sprintf("Detected health change event for Resource: %s. Notifying watcher.", resourceInfo.ResourceID))
			}
		case <-stopCh:
			logger.Info(fmt.Sprintf("Stopped health monitoring for namespace: %v", kID.Namespace))
			return healthcontract.ResourceHealthDataMessage{}
		}
	}
}
