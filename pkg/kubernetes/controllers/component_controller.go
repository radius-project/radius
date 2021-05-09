// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/workloads"
)

// ComponentReconciler reconciles a Component object
type ComponentReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder

	Model model.ApplicationModel
}

//+kubebuilder:rbac:groups=applications.radius.dev,resources=components,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=applications.radius.dev,resources=components/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=applications.radius.dev,resources=components/finalizers,verbs=update

func (r *ComponentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("component", req.NamespacedName)

	component := &radiusv1alpha1.Component{}
	err := r.Get(ctx, req.NamespacedName, component)
	if client.IgnoreNotFound(err) == nil {
		// Component was deleted - we don't need to handle this because it will cascade
	} else if err != nil {
		log.Error(err, "failed to retrieve component")
		return ctrl.Result{}, err
	}

	log = log.WithValues(
		"application", component.Spec.Application,
		"component", component.Spec.Name,
		"componentkind", component.Spec.Kind)

	generic := &components.GenericComponent{}
	err = r.Scheme.Convert(component, generic, ctx)
	if err != nil {
		log.Error(err, "failed to convert component")
		return ctrl.Result{}, err
	}

	componentKind, err := r.Model.LookupComponent(generic.Kind)
	if err != nil {
		log.Error(err, "unsupported kind for component")
		return ctrl.Result{}, err
	}

	w := workloads.InstantiatedWorkload{
		Application:   component.Spec.Application,
		Name:          component.Spec.Name,
		Workload:      *generic,
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := componentKind.Renderer().Render(ctx, w)
	if err != nil {
		log.Error(err, "failed to render resources for component")
		return ctrl.Result{}, err
	}

	log.Info("rendered output resources", "count", len(resources))

	for _, cr := range resources {
		obj, ok := cr.Resource.(client.Object)
		if !ok {
			err := fmt.Errorf("resource is not a kubernetes resource, was: %T", cr.Resource)
			log.Error(err, "failed to render resources for component")
			return ctrl.Result{}, err
		}

		obj.SetNamespace(component.Namespace)
		obj.SetName(fmt.Sprintf("%s-%s", component.Spec.Application, obj.GetName()))

		log := log.WithValues(
			"resourcenamespace", obj.GetNamespace(),
			"resourcename", obj.GetName(),
			"resourcekind", obj.GetObjectKind().GroupVersionKind().String())

		err := controllerutil.SetControllerReference(component, obj, r.Scheme)
		if err != nil {
			log.Error(err, "failed to set owner reference for resource")
			return ctrl.Result{}, err
		}

		log.Info("applying output resource for component")
		err = r.Client.Patch(ctx, obj, client.Apply, client.FieldOwner("radius"))
		if err != nil {
			log.Error(err, "failed to apply resources for component", "kind", generic.Kind)
			return ctrl.Result{}, err
		}

		log.Info("applied output resource for component")
	}

	log.Info("applied output resources", "count", len(resources))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Model = model.NewKubernetesModel(&r.Client)
	r.recorder = mgr.GetEventRecorderFor("radius")

	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha1.Component{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
