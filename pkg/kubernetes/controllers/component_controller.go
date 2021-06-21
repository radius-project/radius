// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

const (
	CacheKeySpecApplication = "metadata.application"
	CacheKeyController      = "metadata.controller"
	AnnotationLocalID       = "radius.dev/local-id"
)

// ComponentReconciler reconciles a Component object
type ComponentReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder

	Model model.ApplicationModel
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="dapr.io",resources=components,verbs=get;watch;list;create;update;patch;delete
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
		"application", component.Annotations["radius.dev/applications"],
		"component", component.Annotations["radius.dev/components"],
		"componentkind", component.Spec.Kind)

	application := &radiusv1alpha1.Application{}
	key := client.ObjectKey{Namespace: component.Namespace, Name: "radius-" + component.Annotations["radius.dev/applications"]}
	err = r.Get(ctx, key, application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// Application is not found
		r.recorder.Eventf(component, "Normal", "Waiting", "Application %s does not exist", component.Annotations["radius.dev/applications"])
		log.Info("application does not exist... waiting")

		// Keep going, we'll turn this into an "empty" render

	} else if err != nil {
		log.Error(err, "failed to retrieve application")
		return ctrl.Result{}, err
	}

	// Now we need to rationalize the set of logical resources (desired state against the actual state)
	actual, err := r.FetchKubernetesResources(ctx, log, component)
	if err != nil {
		return ctrl.Result{}, err
	}

	desired, bindings, rendered, err := r.RenderComponent(ctx, log, application, component, component.Annotations["radius.dev/applications"], component.Annotations["radius.dev/components"])
	if err != nil {
		return ctrl.Result{}, err
	}

	if rendered {
		component.Status.Phrase = "Ready"
	} else {
		component.Status.Phrase = "Waiting"
	}

	err = r.ApplyState(ctx, log, application, component, actual, desired, bindings)
	if err != nil {
		return ctrl.Result{}, err
	}

	if rendered {
		r.recorder.Event(component, "Normal", "Rendered", "Component has been processed successfully")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ComponentReconciler) FetchKubernetesResources(ctx context.Context, log logr.Logger, component *radiusv1alpha1.Component) ([]client.Object, error) {
	log.Info("fetching existing resources for component")
	results := []client.Object{}

	deployments := &appsv1.DeploymentList{}
	err := r.Client.List(ctx, deployments, client.InNamespace(component.Namespace), client.MatchingFields{CacheKeyController: component.Name})
	if err != nil {
		log.Error(err, "failed to retrieve deployments")
		return nil, err
	}

	for _, d := range (*deployments).Items {
		obj := d
		results = append(results, &obj)
	}

	services := &corev1.ServiceList{}
	err = r.Client.List(ctx, services, client.InNamespace(component.Namespace), client.MatchingFields{CacheKeyController: component.Name})
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

// Make this work for generic
func (r *ComponentReconciler) RenderComponent(ctx context.Context, log logr.Logger, application *radiusv1alpha1.Application, component *radiusv1alpha1.Component, applicationName string, componentName string) ([]workloads.OutputResource, []radiusv1alpha1.ComponentStatusBinding, bool, error) {
	// If the application hasn't been defined yet, then just produce no output.
	if application == nil {
		r.recorder.Eventf(component, "Normal", "Waiting", "Component is waiting for application: %s", applicationName)
		return nil, nil, false, nil
	}

	generic := &components.GenericComponent{}
	err := r.Scheme.Convert(component, generic, ctx)
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component could not be converted: %v", err)
		log.Error(err, "failed to convert component")
		return nil, nil, false, err
	}

	componentKind, err := r.Model.LookupComponent(generic.Kind)
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component kind '%s' is not supported'", generic.Kind)
		log.Error(err, "unsupported kind for component")
		return nil, nil, false, err
	}

	w := workloads.InstantiatedWorkload{
		Application:   applicationName,
		Name:          componentName,
		Namespace:     component.Namespace,
		Workload:      *generic,
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	missing := []components.BindingKey{}
	for _, dependency := range generic.Uses {
		key := dependency.Binding.TryGetBindingKey()
		if key == nil {
			continue
		}

		// TODO use an index
		providers := radiusv1alpha1.ComponentList{}
		err := r.Client.List(ctx, &providers, client.InNamespace(component.Namespace))
		if err != nil {
			log.Error(err, "failed to list components")
			return nil, nil, false, err
		}

		found := false
		for _, pp := range providers.Items {
			if pp.Annotations["radius.dev/applications"] != applicationName {
				continue
			}

			if pp.Annotations["radius.dev/components"] != key.Component {
				continue
			}

			// TODO detect duplicates and kind mismatches
			for _, binding := range pp.Status.Bindings {
				if binding.Name == key.Binding {
					values := map[string]interface{}{}
					err := json.Unmarshal(binding.Values.Raw, &values)
					if err != nil {
						log.Error(err, "failed to list components")
						return nil, nil, false, err
					}

					w.BindingValues[*key] = components.BindingState{
						Component:  key.Component,
						Binding:    key.Binding,
						Kind:       binding.Kind,
						Properties: values,
					}
					found = true
					break
				}
			}

			if found {
				break
			}
		}

		if !found {
			missing = append(missing, *key)
		}
	}

	resources := []workloads.OutputResource{}
	if len(missing) > 0 {
		missingNames := []string{}
		for _, key := range missing {
			missingNames = append(missingNames, fmt.Sprintf("%s:%s", key.Component, key.Binding))
		}
		r.recorder.Eventf(component, "Normal", "Waiting", "Component is waiting for bindings: %s", strings.Join(missingNames, ", "))
		log.Info("component is waiting for bindings", "missing", missing)
	} else {
		resources, err = componentKind.Renderer().Render(ctx, w)
		if err != nil {
			r.recorder.Eventf(component, "Warning", "Invalid", "Component had errors during rendering: %v'", err)
			log.Error(err, "failed to render resources for component")
			return nil, nil, false, err
		}
	}

	bindingStates, err := componentKind.Renderer().AllocateBindings(ctx, w, []workloads.WorkloadResourceProperties{})
	if err != nil {
		r.recorder.Eventf(component, "Warning", "Invalid", "Component had errors during rendering: %v'", err)
		log.Error(err, "failed to render bindings for component")
		return nil, nil, false, err
	}

	bindings := []radiusv1alpha1.ComponentStatusBinding{}
	for name, binding := range bindingStates {
		kind := binding.Kind

		b, err := json.Marshal(binding.Properties)
		if err != nil {
			r.recorder.Eventf(component, "Warning", "Invalid", "Component had errors during rendering: %v'", err)
			log.Error(err, "failed to render bindings for component")
			return nil, nil, false, err
		}

		bindings = append(bindings, radiusv1alpha1.ComponentStatusBinding{
			Name:   name,
			Kind:   kind,
			Values: runtime.RawExtension{Raw: b},
		})
	}

	log.Info("rendered output resources", "count", len(resources))
	return resources, bindings, len(missing) == 0, nil
}

func (r *ComponentReconciler) ApplyState(
	ctx context.Context,
	log logr.Logger,
	application *radiusv1alpha1.Application,
	component *radiusv1alpha1.Component,
	actual []client.Object,
	desired []workloads.OutputResource,
	bindings []radiusv1alpha1.ComponentStatusBinding) error {

	// First we go through the desired state and apply all of those resources.
	//
	// While we do that we eliminate items from the 'actual' state list that are part of the desired
	// state. This leaves us with the set of things that need to be deleted
	//
	// We also trample over the 'resources' part of the status so that it's clean.

	component.Status.Resources = map[string]corev1.ObjectReference{}

	for _, cr := range desired {
		obj, ok := cr.Resource.(client.Object)
		if !ok {
			err := fmt.Errorf("resource is not a kubernetes resource, was: %T", cr.Resource)
			log.Error(err, "failed to render resources for component")
			return err
		}

		// TODO: configure all of the metadata at the top-level
		obj.SetNamespace(component.Namespace)
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[AnnotationLocalID] = cr.LocalID
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
			return err
		}

		// We don't have to diff the actual resource - server side apply is magic.
		log.Info("applying output resource for component")
		err = r.Client.Patch(ctx, obj, client.Apply, client.FieldOwner("radius"), client.ForceOwnership)
		if err != nil {
			log.Error(err, "failed to apply resources for component")
			return err
		}

		or, err := ref.GetReference(r.Scheme, obj)
		if err != nil {
			log.Error(err, "failed to get resource reference for resource")
			return err
		}

		component.Status.Resources[cr.LocalID] = *or

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
			return err
		}

		log.Info("deleted unused resource")
	}

	component.Status.Bindings = bindings

	err := r.Status().Update(ctx, component)
	if err != nil {
		log.Error(err, "failed to update resource status for component")
		return err
	}

	log.Info("applied output resources", "count", len(desired), "deleted", len(actual))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Model = model.NewKubernetesModel(&r.Client)
	r.recorder = mgr.GetEventRecorderFor("radius")

	// Index components by application
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &radiusv1alpha1.Component{}, CacheKeySpecApplication, extractApplicationKey)
	if err != nil {
		return err
	}

	// Index deployments by the owner (component)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		return err
	}

	// Index services by the owner (component)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		return err
	}

	cache := mgr.GetClient()
	applicationSource := &source.Kind{Type: &radiusv1alpha1.Application{}}
	applicationHandler := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []ctrl.Request {
		application := obj.(*radiusv1alpha1.Application)
		components := &radiusv1alpha1.ComponentList{}
		err := cache.List(context.Background(), components, client.InNamespace(application.Namespace), client.MatchingFields{CacheKeySpecApplication: application.Name})
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
	return []string{component.Annotations["radius.dev/applications"]}
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
