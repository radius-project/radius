// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/renderers"
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

func (r *DeploymentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("deploymenttemplate", req.NamespacedName)

	arm := &bicepv1alpha3.DeploymentTemplate{}
	err := r.Get(ctx, req.NamespacedName, arm)
	if err != nil {
		return ctrl.Result{}, err
	}

	arm.Status.Operations = nil

	result, err := r.ApplyState(ctx, req, arm)

	_ = r.Status().Update(ctx, arm)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, err
}

// Parses the arm template and deploys individual resources to the cluster
// TODO: Can we avoid parsing resources multiple times by caching?
func (r *DeploymentTemplateReconciler) ApplyState(ctx context.Context, req ctrl.Request, arm *bicepv1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	template, err := armtemplate.Parse(string(arm.Spec.Content.Raw))
	if err != nil {
		return ctrl.Result{}, err
	}

	options := armtemplate.TemplateOptions{
		SubscriptionID: "kubernetes",
		ResourceGroup:  req.Namespace,
	}
	resources, err := armtemplate.Eval(template, options)
	if err != nil {
		return ctrl.Result{}, err
	}

	// All previously deployed resources to be used by other resources
	// to fill in variables ex: ([reference(...)])
	deployed := map[string]map[string]interface{}{}
	evaluator := &armtemplate.DeploymentEvaluator{
		Template:  template,
		Options:   options,
		Deployed:  deployed,
		Variables: map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		if err != nil {
			return ctrl.Result{}, err
		}

		evaluator.Variables[name] = value
	}

	for i, resource := range resources {
		body, err := evaluator.VisitMap(resource.Body)
		if err != nil {
			return ctrl.Result{}, err
		}

		resource.Body = body

		k8sInfo, err := armtemplate.ConvertToK8s(resource, req.NamespacedName.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO track progress of operations (count of deployed resources) in Status.
		arm.Status.Operations = append(arm.Status.Operations, bicepv1alpha3.DeploymentTemplateOperation{
			Name:      k8sInfo.GetName(),
			Namespace: k8sInfo.GetNamespace(),
		})

		err = r.Client.Get(ctx, client.ObjectKey{
			Namespace: k8sInfo.GetNamespace(),
			Name:      k8sInfo.GetName(),
		}, k8sInfo)

		if err != nil && client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if apierrors.IsNotFound(err) {
			err = r.Client.Patch(ctx, k8sInfo, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		arm.Status.Operations[i].Provisioned = true

		// TODO could remove this dependecy on radiusv1alpha3
		k8sResource := &radiusv1alpha3.Resource{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(k8sInfo.Object, k8sResource)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Reference additional properties of the status.
		deployed[resource.ID] = map[string]interface{}{}

		if k8sResource.Status.ComputedValues != nil {
			computedValues := map[string]renderers.ComputedValueReference{}

			err = json.Unmarshal(k8sResource.Status.ComputedValues.Raw, &computedValues)
			if err != nil {
				return ctrl.Result{}, err
			}

			for key, value := range computedValues {
				deployed[resource.ID][key] = value.Value
			}
		}

		// transform from k8s representation to arm representation

		if k8sResource.Status.Phrase != "Ready" {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bicepv1alpha3.DeploymentTemplate{}).
		Complete(r)
}
