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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

func NewKubernetesServiceHandler(k8s kubernetes.Interface) HealthHandler {
	return &kubernetesServiceHandler{k8s: k8s}
}

type kubernetesServiceHandler struct {
	k8s kubernetes.Interface
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
			ResourceKind: resourcekinds.KindKubernetes,
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
	if err != nil {
		msg := healthcontract.ResourceHealthDataMessage{
			Resource: healthcontract.ResourceInfo{
				HealthID:     resourceInfo.HealthID,
				ResourceID:   resourceInfo.ResourceID,
				ResourceKind: resourcekinds.KindKubernetes,
			},
			HealthState:             healthcontract.HealthStateUnhealthy,
			HealthStateErrorDetails: err.Error(),
		}
		options.WatchHealthChangesChannel <- msg
		return msg
	}
	defer watcher.Stop()

	svcChans := watcher.ResultChan()

	for {
		state := ""
		detail := ""
		select {
		case svcEvent := <-svcChans:
			switch svcEvent.Type {
			case watch.Deleted:
				state = healthcontract.HealthStateUnhealthy
				detail = "Service deleted"
			case watch.Added:
			case watch.Modified:
				state = healthcontract.HealthStateHealthy
				detail = ""
			}
			// Notify the watcher. Let the watcher determine if an action is needed
			msg := healthcontract.ResourceHealthDataMessage{
				Resource: healthcontract.ResourceInfo{
					HealthID:     resourceInfo.HealthID,
					ResourceID:   resourceInfo.ResourceID,
					ResourceKind: resourcekinds.KindKubernetes,
				},
				HealthState:             state,
				HealthStateErrorDetails: detail,
			}
			options.WatchHealthChangesChannel <- msg
			logger.Info(fmt.Sprintf("Detected health change event for Resource: %s. Notifying watcher.", resourceInfo.ResourceID))
		case <-options.StopChannel:
			logger.Info(fmt.Sprintf("Stopped health monitoring for namespace: %v", kID.Namespace))
			return healthcontract.ResourceHealthDataMessage{}
		}
	}
}
