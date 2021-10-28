// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=radius.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=applications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=applications/finalizers,verbs=update

func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("application", req.NamespacedName)
	app := &radiusv1alpha3.Application{}
	err := r.Client.Get(ctx, req.NamespacedName, app)
	if err != nil {
		return ctrl.Result{}, err
	}

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		Message:            "Application is ready.",
		ObservedGeneration: app.Generation,
	}

	meta.SetStatusCondition(&app.Status.Conditions, condition)

	app.Status.ObservedGeneration = app.Generation

	err = r.Status().Update(ctx, app)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha3.Application{}).
		Complete(r)
}
