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
	"github.com/Azure/radius/pkg/resourcemodel"
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

func (handler *kubernetesDeploymentHandler) GetHealthState(ctx context.Context, registration HealthRegistration, options Options) HealthState {
	kID := registration.Identity.Data.(resourcemodel.KubernetesIdentity)
	logger := radlogger.GetLogger(ctx)

	pod, err := handler.k8s.CoreV1().Pods(kID.Namespace).Get(ctx, kID.Name, metav1.GetOptions{})
	var healthState string
	var healthStateErrorDetails string
	if err != nil {
		healthState = healthcontract.HealthStateUnhealthy
		healthStateErrorDetails = err.Error()
	} else if pod.Status.Phase == corev1.PodRunning {
		healthState = healthcontract.HealthStateHealthy
		healthStateErrorDetails = ""
	} else {
		healthState = healthcontract.HealthStateUnhealthy
		healthStateErrorDetails = pod.Status.Reason
	}

	// Notify initial health state transition. This needs to be done explicitly since
	// the pod might already be up and running when the health is first probed and the watcher
	// will not detect the initial transition
	msg := HealthState{
		Registration:            registration,
		HealthState:             healthState,
		HealthStateErrorDetails: healthStateErrorDetails,
	}
	options.WatchHealthChangesChannel <- msg
	logger.Info(fmt.Sprintf("Detected health change event for Resource: %+v. Notifying watcher.", registration.Identity))

	// Start watching deployment changes
	w, err := handler.k8s.CoreV1().Pods(kID.Namespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		LabelSelector: fmt.Sprintf("%s=%s", KubernetesLabelName, kID.Name),
	})
	if err != nil {
		msg := HealthState{
			Registration:            registration,
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
			msg := HealthState{
				Registration:            registration,
				HealthState:             healthState,
				HealthStateErrorDetails: healthStateErrorDetails,
			}
			options.WatchHealthChangesChannel <- msg
			logger.Info(fmt.Sprintf("Detected health change event for Resource: %+v. Notifying watcher.", registration.Identity))
		case <-options.StopChannel:
			logger.Info(fmt.Sprintf("Stopped health monitoring for namespace: %s", kID.Namespace))
			return HealthState{}
		}
	}
}
