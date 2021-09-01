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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8s "k8s.io/client-go/kubernetes"
)

func NewKubernetesServiceHandler(k8s k8s.Clientset) HealthHandler {
	return &kubernetesServiceHandler{k8s: k8s}
}

type kubernetesServiceHandler struct {
	k8s k8s.Clientset
}

func (handler *kubernetesServiceHandler) GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options handleroptions.Options) healthcontract.ResourceHealthDataMessage {
	logger := radlogger.GetLogger(ctx)
	kID, err := healthcontract.ParseK8sResourceID(resourceInfo.ResourceID)
	if err != nil {
		return healthcontract.ResourceHealthDataMessage{
			Resource:                resourceInfo,
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
	}

	var healthState string
	var healthStateErrorDetails string

	// Only checking service existence to mark status as healthy
	_, err = handler.k8s.CoreV1().Services(kID.Namespace).Get(ctx, kID.Name, metav1.GetOptions{})
	if err != nil {
		healthState = healthcontract.HealthStateUnhealthy
		healthStateErrorDetails = err.Error()
	} else {
		healthState = healthcontract.HealthStateHealthy
		healthStateErrorDetails = ""
	}

	// Notify initial health state transition. This needs to be done explicitly since
	// the service might already exist when the health is first probed and the watcher
	// will not detect the initial transition
	msg := healthcontract.ResourceHealthDataMessage{
		Resource: healthcontract.ResourceInfo{
			HealthID:     resourceInfo.HealthID,
			ResourceID:   resourceInfo.ResourceID,
			ResourceKind: "ResourceKindKubernetes",
		},
		HealthState:             healthState,
		HealthStateErrorDetails: healthStateErrorDetails,
	}
	options.WatchHealthChangesChannel <- msg
	logger.Info(fmt.Sprintf("Detected health change event for Resource: %s. Notifying watcher.", resourceInfo.ResourceID))

	// Now watch for changes to the service
	watcher, err := handler.k8s.CoreV1().Services(kID.Namespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		LabelSelector: fmt.Sprintf("%s=%s", KubernetesLabelName, kID.Name),
	})
	svcChans := watcher.ResultChan()

	for {
		state := ""
		detail := ""
		select {
		case svcEvent := <-svcChans:
			_, ok := svcEvent.Object.(*corev1.Service)
			if !ok {
				state = healthcontract.HealthStateUnhealthy
				detail = "Object is not a service"
			} else {
				switch svcEvent.Type {
				case watch.Deleted:
					state = healthcontract.HealthStateUnhealthy
					detail = "Service deleted"
				case watch.Added:
				case watch.Modified:
					state = healthcontract.HealthStateHealthy
					detail = ""
				}
			}
			// Health state has changed. Notify the watcher
			if state != "" {
				msg := healthcontract.ResourceHealthDataMessage{
					Resource: healthcontract.ResourceInfo{
						HealthID:     resourceInfo.HealthID,
						ResourceID:   resourceInfo.ResourceID,
						ResourceKind: "ResourceKindKubernetes",
					},
					HealthState:             state,
					HealthStateErrorDetails: detail,
				}
				options.WatchHealthChangesChannel <- msg
				logger.Info(fmt.Sprintf("Detected health change event for Resource: %s. Notifying watcher.", resourceInfo.ResourceID))
			}
		case <-options.StopCh:
			logger.Info(fmt.Sprintf("Stopped health monitoring for namespace: %v", kID.Namespace))
			return healthcontract.ResourceHealthDataMessage{}
		}
	}
}
