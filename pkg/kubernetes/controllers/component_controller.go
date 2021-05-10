// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"
	"strings"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const ApplicationKey = "spec.application"
const OwnerKey = "metadata.controller"

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
	if err != nil && client.IgnoreNotFound(err) == nil {
		// Component was deleted - we don't need to handle this because it will cascade
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to retrieve component")
		return ctrl.Result{}, err
	}

	log = log.WithValues(
		"application", component.Spec.Application,
		"component", component.Spec.Name,
		"componentkind", component.Spec.Kind)

	application := &radiusv1alpha1.Application{}
	key := client.ObjectKey{Namespace: component.Namespace, Name: component.Spec.Application}
	err = r.Get(ctx, key, application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// Application is not found
		r.recorder.Eventf(component, "Normal", "Waiting", "Application %s does not exist", component.Spec.Application)
		log.Info("application does not exist... waiting")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to retrieve application")
		return ctrl.Result{}, err
	}

	// Now we need to rationalize the set of logical resources (desired state against the actual state)
	actual, err := r.FetchActualResources(ctx, log, component)
	if err != nil {
		return ctrl.Result{}, err
	}

	desired, err := r.RenderComponent(ctx, log, component)
	if err != nil {
		return ctrl.Result{}, err
	}

	// First we go through the desired state and apply all of those resources.
	//
	// While we do that we eliminate items from the 'actual' state list that are part of the desired
	// state. This leaves us with the set of things that need to be deleted
	//
	// We also trample over the 'resources' part of the status so that it's clean.

	statuschanged := false
	oldstatus := component.Status.Resources
	if oldstatus == nil {
		oldstatus = map[string]corev1.ObjectReference{}
	}
	component.Status.Resources = map[string]corev1.ObjectReference{}

	for _, cr := range desired {
		obj, ok := cr.Resource.(client.Object)
		if !ok {
			err := fmt.Errorf("resource is not a kubernetes resource, was: %T", cr.Resource)
			log.Error(err, "failed to render resources for component")
			return ctrl.Result{}, err
		}

		obj.SetNamespace(component.Namespace)
		obj.SetName(fmt.Sprintf("%s-%s-%s", component.Spec.Application, obj.GetName(), strings.ToLower(cr.LocalID)))
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["radius.dev/local-id"] = cr.LocalID
		obj.SetAnnotations(annotations)

		// Remove items with the same identity from the 'actual' list
		for i, a := range actual {
			if a.GetObjectKind().GroupVersionKind().String() == obj.GetObjectKind().GroupVersionKind().String() && a.GetName() == obj.GetName() && a.GetNamespace() == obj.GetNamespace() {
				actual = append(actual[:i], actual[i+1:]...)
				break
			}
		}

		log := log.WithValues(
			"resourcenamespace", obj.GetNamespace(),
			"resourcename", obj.GetName(),
			"resourcekind", obj.GetObjectKind().GroupVersionKind().String(),
			"localid", cr.LocalID)

		err := controllerutil.SetControllerReference(component, obj, r.Scheme)
		if err != nil {
			log.Error(err, "failed to set owner reference for resource")
			return ctrl.Result{}, err
		}

		// We don't have to diff the actual resource - server side apply is magic.
		log.Info("applying output resource for component")
		err = r.Client.Patch(ctx, obj, client.Apply, client.FieldOwner("radius"), client.ForceOwnership)
		if err != nil {
			log.Error(err, "failed to apply resources for component")
			return ctrl.Result{}, err
		}

		or, err := ref.GetReference(r.Scheme, obj)
		if err != nil {
			log.Error(err, "failed to get resource reference for resource")
			return ctrl.Result{}, err
		}

		component.Status.Resources[cr.LocalID] = *or
		if oldstatus[cr.LocalID].UID != or.UID {
			statuschanged = true
		}

		log.Info("applied output resource for component")
	}

	for _, obj := range actual {
		log := log.WithValues(
			"resourcenamespace", obj.GetNamespace(),
			"resourcename", obj.GetName(),
			"resourcekind", obj.GetObjectKind().GroupVersionKind().String())
		log.Info("deleting unused resource")

		err := r.Client.Delete(ctx, obj)
		if err != nil && client.IgnoreNotFound(err) == nil {
			// ignore
		} else if err != nil {
			log.Error(err, "failed to delete resource for component")
			return ctrl.Result{}, err
		}

		log.Info("deleted unused resource")
	}

	if statuschanged || len(oldstatus) != len(component.Status.Resources) {
		err = r.Status().Update(ctx, component)
		if err != nil {
			log.Error(err, "failed to update resource status for component")
			return ctrl.Result{}, err
		}
	}

	log.Info("applied output resources", "count", len(desired), "deleted", len(actual))
	r.recorder.Event(component, "Normal", "Rendered", "Component has been processed successfully")

	return ctrl.Result{}, nil
}

func (r *ComponentReconciler) FetchActualResources(ctx context.Context, log logr.Logger, component *radiusv1alpha1.Component) ([]client.Object, error) {
	log.Info("fetching existing resources for component")
	results := []client.Object{}

	deployments := &appsv1.DeploymentList{}
	err := r.Client.List(ctx, deployments, client.InNamespace(component.Namespace), client.MatchingFields{OwnerKey: component.Name})
	if err != nil {
		log.Error(err, "failed to retrieve deployments")
		return nil, err
	}

	for _, d := range (*deployments).Items {
		obj := d
		results = append(results, &obj)
	}

	services := &corev1.ServiceList{}
	err = r.Client.List(ctx, services, client.InNamespace(component.Namespace), client.MatchingFields{OwnerKey: component.Name})
	if err != nil {
		log.Error(err, "failed to retrieve services")
		return nil, err
	}

	for _, s := range (*services).Items {
		obj := s
		results = append(results, &obj)
	}

	log.Info("found existing resource for component", "count", len(results))
	return results, nil
}

func (r *ComponentReconciler) RenderComponent(ctx context.Context, log logr.Logger, component *radiusv1alpha1.Component) ([]workloads.WorkloadResource, error) {
	generic := &components.GenericComponent{}
	err := r.Scheme.Convert(component, generic, ctx)
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component could not be converted: %v", err)
		log.Error(err, "failed to convert component")
		return nil, err
	}

	componentKind, err := r.Model.LookupComponent(generic.Kind)
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component kind '%s' is not supported'", generic.Kind)
		log.Error(err, "unsupported kind for component")
		return nil, err
	}

	w := workloads.InstantiatedWorkload{
		Application:   component.Spec.Application,
		Name:          component.Spec.Name,
		Workload:      *generic,
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := componentKind.Renderer().Render(ctx, w)
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component had errors during rendering: %v'", err)
		log.Error(err, "failed to render resources for component")
		return nil, err
	}

	log.Info("rendered output resources", "count", len(resources))
	return resources, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Model = model.NewKubernetesModel(&r.Client)
	r.recorder = mgr.GetEventRecorderFor("radius")

	// Index components by application
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &radiusv1alpha1.Component{}, ApplicationKey, extractOwnerKey)
	if err != nil {
		return err
	}

	// Index deployments by the owner (component)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, OwnerKey, extractOwnerKey)
	if err != nil {
		return err
	}

	// Index services by the owner (component)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, OwnerKey, extractOwnerKey)
	if err != nil {
		return err
	}

	cache := mgr.GetClient()
	applicationSource := &source.Kind{Type: &radiusv1alpha1.Application{}}
	applicationHandler := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []ctrl.Request {
		application := obj.(*radiusv1alpha1.Application)
		components := &radiusv1alpha1.ComponentList{}
		err := cache.List(context.Background(), components, client.InNamespace(application.Namespace), client.MatchingFields{ApplicationKey: application.Name})
		if err != nil {
			mgr.GetLogger().Error(err, "failed to list components")
			return nil
		}

		requests := []ctrl.Request{}
		for _, c := range (*components).Items {
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: application.Namespace, Name: c.Name}})
		}
		return requests
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha1.Component{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(applicationSource, applicationHandler).
		Complete(r)
}

func extractApplicationKey(obj client.Object) []string {
	component := obj.(*radiusv1alpha1.Component)
	return []string{component.Spec.Application}
}

func extractOwnerKey(obj client.Object) []string {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return nil
	}

	if owner.APIVersion != radiusv1alpha1.GroupVersion.String() || owner.Kind != "Component" {
		return nil
	}

	return []string{owner.Name}
}
