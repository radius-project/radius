// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/kubernetes"
	radruntime "github.com/project-radius/radius/pkg/kubernetes/api/radius/runtime/v1alpha3"
	"github.com/project-radius/radius/pkg/kubernetes/converters"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups=runtime.radius.dev,resources=containers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=runtime.radius.dev,resources=containers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=runtime.radius.dev,resources=containers/finalizers,verbs=update;patch;delete

type ContainerController struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	Dynamic    dynamic.Interface
	RestMapper meta.RESTMapper
}

func (r *ContainerController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)

	container := radruntime.Container{}
	err := r.Get(ctx, req.NamespacedName, &container)
	if errors.IsNotFound(err) {
		// Could be deleted after we got the notification
		log.Info("container was deleted")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	log = log.WithValues(
		"application", container.Spec.ApplicationName,
		"resourceName", container.Spec.ResourceName,
		"resourceType", container.Spec.ResourceType)

	deployment := converters.ContainerToDeployment(&container)
	err = controllerutil.SetControllerReference(&container, deployment, r.Scheme)
	if err != nil {
		r.Recorder.Eventf(&container, corev1.EventTypeWarning, "Failed", "Failed to set owner: %s", err)
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, deployment, client.Apply, client.FieldOwner(kubernetes.FieldManager))
	if err != nil {
		r.Recorder.Eventf(&container, corev1.EventTypeWarning, "Failed", "Failed to reconcile deployment: %s", err)
		return ctrl.Result{}, err
	}

	// TODO status

	log.Info("Successfully reconciled container")
	r.Recorder.Event(&container, corev1.EventTypeNormal, "Succeeded", "Successfully reconciled deployment")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ContainerController) SetupWithManager(mgr ctrl.Manager) error {
	c := ctrl.NewControllerManagedBy(mgr).
		For(&radruntime.Container{}).
		Owns(&appsv1.Deployment{})
	return c.Complete(r)
}
