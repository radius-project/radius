// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	radruntime "github.com/project-radius/radius/pkg/kubernetes/api/radius/runtime/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"k8s.io/cri-api/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	log.Info("Hello there")

	container := radruntime.Container{}
	err := r.Get(ctx, req.NamespacedName, &container)
	if errors.IsNotFound(err) {
		// Could be deleted after we got the notification
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ContainerController) SetupWithManager(mgr ctrl.Manager) error {
	c := ctrl.NewControllerManagedBy(mgr).
		For(&radruntime.Container{}).
		Owns(&appsv1.Deployment{})
	return c.Complete(r)
}
