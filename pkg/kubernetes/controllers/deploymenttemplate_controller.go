// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
)

// DeploymentTemplateReconciler reconciles a Arm object
type DeploymentTemplateReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Arm object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *DeploymentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("arm", req.NamespacedName)

	arm := &radiusv1alpha1.DeploymentTemplate{}
	err := r.Get(ctx, req.NamespacedName, arm)
	if err != nil {
		return ctrl.Result{}, err
	}

	template, err := armtemplate.Parse(string(arm.Spec.Content.Raw))
	if err != nil {
		return ctrl.Result{}, err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{})
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, resource := range resources {
		k8sInfo, err := armtemplate.ConvertToK8s(resource, req.NamespacedName.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}

		r.Client.Patch(ctx, k8sInfo.Unstructured, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha1.DeploymentTemplate{}).
		Complete(r)
}
