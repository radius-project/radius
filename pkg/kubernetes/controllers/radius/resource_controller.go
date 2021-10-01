// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	k8smodel "github.com/Azure/radius/pkg/model/kubernetes"
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	CacheKeySpecApplication = "metadata.application"
	CacheKeyController      = "metadata.controller"
	AnnotationLocalID       = "radius.dev/local-id"
)

// ResourceReconciler reconciles a Resource object
type ResourceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
	Dynamic  dynamic.Interface
	Model    model.ApplicationModel
	GVR      schema.GroupVersionResource
}

//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="dapr.io",resources=components,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=resources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=resources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=resources/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=daprioinvokeroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=daprioinvokeroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=daprioinvokeroutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=mongodbcomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=mongodbcomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=mongodbcomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=rediscomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=rediscomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=rediscomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=grpcroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=grpcroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=grpcroutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=dapriopubsubtopiccomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=dapriopubsubtopiccomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=dapriopubsubtopiccomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=rabbitmqcomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=rabbitmqcomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=rabbitmqcomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=httproutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=dapriostatestorecomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=dapriostatestorecomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=dapriostatestorecomponents/finalizers,verbs=update

func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("resource", req.NamespacedName)

	unst, err := r.Dynamic.Resource(r.GVR).Namespace(req.Namespace).Get(ctx, req.Name, v1.GetOptions{})
	if err != nil {
		return ctrl.Result{}, err
	}

	resource := &radiusv1alpha3.Resource{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unst.Object, resource)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// Resource was deleted - we don't need to handle this because it will cascade
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to retrieve resource")
		return ctrl.Result{}, err
	}

	applicationName := resource.Annotations[kubernetes.LabelRadiusApplication]
	resourceName := resource.Annotations[kubernetes.LabelRadiusResource]

	log = log.WithValues(
		"application", applicationName,
		"resource", resourceName)

	application := &radiusv1alpha3.Application{}
	key := client.ObjectKey{Namespace: resource.Namespace, Name: applicationName}
	err = r.Get(ctx, key, application)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// Application is not found
		r.recorder.Eventf(resource, "Normal", "Waiting", "Application %s does not exist", applicationName)
		log.Info("application does not exist... waiting")

		// Keep going, we'll turn this into an "empty" render

	} else if err != nil {
		log.Error(err, "failed to retrieve application")
		return ctrl.Result{}, err
	}

	desired, rendered, err := r.RenderResource(ctx, req, log, application, resource, applicationName, resourceName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if rendered {
		resource.Status.Phrase = "Ready"
	} else {
		resource.Status.Phrase = "Waiting"
	}

	// Now we need to rationalize the set of logical resources (desired state against the actual state)
	actual, err := r.FetchKubernetesResources(ctx, log, resource)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ApplyState(ctx, log, req, application, resource, unst, actual, *desired)
	if err != nil {
		return ctrl.Result{}, err
	}

	if rendered {
		r.recorder.Event(resource, "Normal", "Rendered", "Resource has been processed successfully")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ResourceReconciler) FetchKubernetesResources(ctx context.Context, log logr.Logger, resource *radiusv1alpha3.Resource) ([]client.Object, error) {
	log.Info("fetching existing resources for resource")
	results := []client.Object{}

	deployments := &appsv1.DeploymentList{}
	err := r.Client.List(ctx, deployments, client.InNamespace(resource.Namespace), client.MatchingFields{CacheKeyController: resource.Name})
	if err != nil {
		log.Error(err, "failed to retrieve deployments")
		return nil, err
	}

	for _, d := range (*deployments).Items {
		obj := d
		results = append(results, &obj)
	}

	services := &corev1.ServiceList{}
	err = r.Client.List(ctx, services, client.InNamespace(resource.Namespace), client.MatchingFields{CacheKeyController: resource.Name})
	if err != nil {
		log.Error(err, "failed to retrieve services")
		return nil, err
	}

	for _, s := range (*services).Items {
		obj := s
		results = append(results, &obj)
	}

	log.Info("found existing resource for resource", "count", len(results))
	return results, nil
}

func (r *ResourceReconciler) RenderResource(ctx context.Context, req ctrl.Request, log logr.Logger, application *radiusv1alpha3.Application, resource *radiusv1alpha3.Resource, applicationName string, resourceName string) (*renderers.RendererOutput, bool, error) {
	// If the application hasn't been defined yet, then just produce no output.
	if application == nil {
		r.recorder.Eventf(resource, "Normal", "Waiting", "Resource is waiting for application: %s", applicationName)
		return nil, false, nil
	}

	w := &renderers.RendererResource{}
	err := converters.ConvertToRenderResource(resource, w)
	if err != nil {
		r.recorder.Eventf(resource, "Warning", "Invalid", "Resource could not be converted: %v", err)
		log.Error(err, "failed to convert resource")
		return nil, false, err
	}

	resourceType, err := r.Model.LookupResource(w.ResourceType)
	if err != nil {
		r.recorder.Eventf(resource, "Warning", "Invalid", "Resource type '%s' is not supported'", w.ResourceType)
		log.Error(err, "unsupported type for resource")
		return nil, false, err
	}

	references, err := resourceType.Renderer().GetDependencyIDs(ctx, *w)
	if err != nil {
		r.recorder.Eventf(resource, "Warning", "Invalid", "Resource could not get dependencies: %v", err)
		log.Error(err, "failed to render resource")
		return nil, false, err
	}

	deps := map[string]renderers.RendererDependency{}

	for _, reference := range references {
		// Get resource filtered on application type.
		resourceType := reference.Types[len(reference.Types)-1]
		unst := &unstructured.Unstructured{}

		// TODO determine this correctly
		unst.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "radius.dev",
			Version: "v1alpha3",
			Kind:    armtemplate.GetKindFromArmType(resourceType.Type),
		})

		err = r.Client.Get(ctx, client.ObjectKey{
			Namespace: req.Namespace,
			Name:      resourceType.Name,
		}, unst)
		if err != nil {
			// TODO make this wait without an error?
			return nil, false, err
		}

		k8sResource := &radiusv1alpha3.Resource{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unst.Object, k8sResource)
		if err != nil {
			return nil, false, err
		}

		computedValues := map[string]interface{}{}

		err = json.Unmarshal(k8sResource.Status.ComputedValues.Raw, &computedValues)
		if err != nil {
			return nil, false, err
		}

		deps[reference.ID] = renderers.RendererDependency{
			ComputedValues: computedValues,
			ResourceID:     reference,
			Definition:     unst.Object,
		}
	}

	resources, err := resourceType.Renderer().Render(ctx, *w, deps)
	if err != nil {
		r.recorder.Eventf(resource, "Warning", "Invalid", "Resource had errors during rendering: %v'", err)
		log.Error(err, "failed to render resources for resource")
		return nil, false, err
	}

	log.Info("rendered output resources", "count", len(resources.Resources))
	return &resources, true, nil
}

func (r *ResourceReconciler) ApplyState(
	ctx context.Context,
	log logr.Logger,
	req ctrl.Request,
	application *radiusv1alpha3.Application,
	resource *radiusv1alpha3.Resource,
	inputUnst *unstructured.Unstructured,
	actual []client.Object,
	desired renderers.RendererOutput) error {

	// First we go through the desired state and apply all of those resources.
	//
	// While we do that we eliminate items from the 'actual' state list that are part of the desired
	// state. This leaves us with the set of things that need to be deleted
	//
	// We also trample over the 'resources' part of the status so that it's clean.

	resource.Status.Resources = map[string]corev1.ObjectReference{}

	for _, cr := range desired.Resources {
		obj, ok := cr.Resource.(client.Object)
		if !ok {
			err := fmt.Errorf("resource is not a kubernetes resource, was: %T", cr.Resource)
			log.Error(err, "failed to render resources for resource")
			return err
		}

		// TODO: configure all of the metadata at the top-level
		obj.SetNamespace(resource.Namespace)
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

		// Make sure to NOT use the resource type here, as the resource type
		// Otherwise, we get into a loop where resources are created and are immediately terminated.
		err := controllerutil.SetControllerReference(inputUnst, obj, r.Scheme)
		if err != nil {
			log.Error(err, "failed to set owner reference for resource")
			return err
		}

		// We don't have to diff the actual resource - server side apply is magic.
		log.Info("applying output resource for resource")
		err = r.Client.Patch(ctx, obj, client.Apply, client.FieldOwner("radius"), client.ForceOwnership)
		if err != nil {
			log.Error(err, "failed to apply resources for resource")
			return err
		}

		or, err := ref.GetReference(r.Scheme, obj)
		if err != nil {
			log.Error(err, "failed to get resource reference for resource")
			return err
		}

		resource.Status.Resources[cr.LocalID] = *or

		log.Info("applied output resource for resource")
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
			log.Error(err, "failed to delete resource for resource")
			return err
		}

		log.Info("deleted unused resource")
	}

	// Only support strings for now
	if desired.ComputedValues != nil {
		data, err := json.Marshal(desired.ComputedValues)
		if err != nil {
			return err
		}
		// TODO convert from computed value to to interface{}
		resource.Status.ComputedValues = &runtime.RawExtension{Raw: data}
	}

	// Can't use resource type to update as it will assume the wrong type
	unst, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
	if err != nil {
		return err
	}

	u := &unstructured.Unstructured{Object: unst}

	_, err = r.Dynamic.Resource(r.GVR).Namespace(req.Namespace).UpdateStatus(ctx, u, v1.UpdateOptions{})

	if err != nil {
		log.Error(err, "failed to update resource status for resource")
		return err
	}

	log.Info("applied output resources", "count", len(desired.Resources), "deleted", len(actual))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceReconciler) SetupWithManager(mgr ctrl.Manager, object client.Object, listObject client.ObjectList) error {
	r.Model = k8smodel.NewKubernetesModel(&r.Client)
	r.recorder = mgr.GetEventRecorderFor("radius")

	// Index resources by application
	err := mgr.GetFieldIndexer().IndexField(context.Background(), object, CacheKeySpecApplication, extractApplicationKey)
	if err != nil {
		return err
	}

	cache := mgr.GetClient()
	applicationSource := &source.Kind{Type: &radiusv1alpha3.Application{}}
	applicationHandler := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []ctrl.Request {
		// Queue notification on each resource when the application changes.
		application := obj.(*radiusv1alpha3.Application)
		err := cache.List(context.Background(), listObject, client.InNamespace(application.Namespace), client.MatchingFields{CacheKeySpecApplication: application.Name})
		if err != nil {
			mgr.GetLogger().Error(err, "failed to list resources")
			return nil
		}

		requests := []ctrl.Request{}
		err = meta.EachListItem(listObject, func(obj runtime.Object) error {
			o := obj.(client.Object)
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: application.Namespace, Name: o.GetName()}})
			return nil
		})
		if err != nil {
			mgr.GetLogger().Error(err, "failed to create requests")
			return nil
		}
		return requests
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(object).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(applicationSource, applicationHandler).
		Complete(r)
}

func extractApplicationKey(obj client.Object) []string {
	return []string{obj.GetAnnotations()[kubernetes.LabelRadiusApplication]}
}
