// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const KubernetesLabelName = "app.kubernetes.io/name"
const DefaultResyncInterval = time.Minute * 10
const (
	DeploymentEventAdd    = "Add"
	DeploymentEventUpdate = "Update"
	DeploymentEventDelete = "Delete"
)

func NewKubernetesDeploymentHandler(k8s kubernetes.Interface) HealthHandler {
	return &kubernetesDeploymentHandler{k8s: k8s}
}

type kubernetesDeploymentHandler struct {
	k8s kubernetes.Interface
}

func GetHealthStateFromDeploymentStatus(d *appsv1.Deployment) (string, string) {
	healthState := healthcontract.HealthStateUnhealthy
	healthStateErrorDetails := "Deployment condition unknown"
	for _, c := range d.Status.Conditions {
		// When the deployment is healthy, the DeploymentAvailable condition has a status True
		if c.Type == appsv1.DeploymentAvailable {
			if c.Status == v1.ConditionTrue {
				healthState = healthcontract.HealthStateHealthy
				healthStateErrorDetails = ""
			} else {
				healthState = healthcontract.HealthStateUnhealthy
				healthStateErrorDetails = c.Reason
			}
			break
		}
	}
	return healthState, healthStateErrorDetails
}

func (handler *kubernetesDeploymentHandler) GetHealthState(ctx context.Context, registration HealthRegistration, options Options) HealthState {
	kID := registration.Identity.Data.(resourcemodel.KubernetesIdentity)
	var healthState string
	var healthStateErrorDetails string
	deploycl := handler.k8s.AppsV1().Deployments(kID.Namespace)
	d, err := deploycl.Get(ctx, kID.Name, metav1.GetOptions{})
	if err != nil {
		healthState = healthcontract.HealthStateUnhealthy
		healthStateErrorDetails = err.Error()
	} else {
		healthState, healthStateErrorDetails = GetHealthStateFromDeploymentStatus(d)
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

	informerFactory := informers.NewSharedInformerFactoryWithOptions(handler.k8s, DefaultResyncInterval, informers.WithNamespace(kID.Namespace))

	deploymentInformer := informerFactory.Apps().V1().Deployments().Informer()
	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			onDeploymentEvent(ctx, DeploymentEventAdd, obj, registration, options.WatchHealthChangesChannel)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			onDeploymentEvent(ctx, DeploymentEventUpdate, newObj, registration, options.WatchHealthChangesChannel)
		},
		DeleteFunc: func(obj interface{}) {
			onDeploymentEvent(ctx, DeploymentEventDelete, obj, registration, options.WatchHealthChangesChannel)
		},
	})
	deploymentInformer.Run(ctx.Done())
	return HealthState{}
}

func onDeploymentEvent(ctx context.Context, event string, obj interface{}, registration HealthRegistration, watchHealthChangesChannel chan<- HealthState) {
	logger := radlogger.GetLogger(ctx)
	deployment := obj.(*appsv1.Deployment)

	// Ignore events that are not meant for the current deployment
	identity := registration.Identity.Data.(resourcemodel.KubernetesIdentity)
	if deployment.Name != identity.Name {
		return
	}
	logger.Info(fmt.Sprintf("Detected health change event %s for Deployment: %+v. Notifying watcher.", event, registration.Identity))
	var healthState string
	var healthStateErrorDetails string
	switch event {
	case DeploymentEventAdd:
	case DeploymentEventUpdate:
		healthState, healthStateErrorDetails = GetHealthStateFromDeploymentStatus(deployment)
	case DeploymentEventDelete:
		healthState = healthcontract.HealthStateUnhealthy
		healthStateErrorDetails = "Deployment deleted"
	default:
		// We do not expect to see any other event
		return
	}
	msg := HealthState{
		Registration:            registration,
		HealthState:             healthState,
		HealthStateErrorDetails: healthStateErrorDetails,
	}
	watchHealthChangesChannel <- msg
}
