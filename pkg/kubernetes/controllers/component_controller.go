// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/radius/pkg/curp/components"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/workloads"
)

// ComponentReconciler reconciles a Component object
type ComponentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

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

	generic, err := r.convert(component)
	if err != nil {
		log.Error(err, "failed to convert component")
		return ctrl.Result{}, err
	}

	log.Info("here's run!", "run", generic.Run)

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

	for _, cr := range resources {
		obj, ok := cr.Resource.(client.Object)
		if !ok {
			err := fmt.Errorf("resource is not a kubernetes resource, was: %T", cr.Resource)
			log.Error(err, "failed to render resources for component")
			return ctrl.Result{}, err
		}

		obj.SetNamespace(component.Namespace)
		obj.SetName(fmt.Sprintf("%s-%s", component.Spec.Application, obj.GetName()))

		log.Info(
			"applying output resource for component",
			"resourcenamespace", obj.GetNamespace(),
			"resourcename", obj.GetName(),
			"resourcekind", obj.GetObjectKind().GroupVersionKind().String())
		err = r.Client.Patch(ctx, obj, client.Apply, client.FieldOwner("radius"))
		if err != nil {
			log.Error(err, "failed to apply resources for component", "kind", generic.Kind)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Model = model.NewKubernetesModel(&r.Client)

	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha1.Component{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *ComponentReconciler) convert(original *radiusv1alpha1.Component) (*components.GenericComponent, error) {
	// TODO make conversions work
	original.Spec.Run.MarshalJSON()

	b, err := json.Marshal(uns["spec"])
	if err != nil {
		return nil, err
	}

	r.Log.Info("here's JSON!", "json", string(b))

	result := components.GenericComponent{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
