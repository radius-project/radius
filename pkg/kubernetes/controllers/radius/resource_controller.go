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

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-logr/logr"
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
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	AnnotationLocalID = "radius.dev/local-id"
)

// ResourceReconciler reconciles a Resource object
type ResourceReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Dynamic      dynamic.Interface
	RestMapper   meta.RESTMapper
	ObjectType   client.Object
	ObjectList   client.ObjectList
	Model        model.ApplicationModel
	GVR          schema.GroupVersionResource
	WatchedTypes []struct {
		client.Object
		client.ObjectList
	}
}

//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="dapr.io",resources=components,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.x-k8s.io",resources=gateways,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.x-k8s.io",resources=gatewayclasses,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.x-k8s.io",resources=httproutes,verbs=get;watch;list;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=resources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=resources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=resources/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=containercomponents/finalizers,verbs=update
//+kubebuilder:rbac:groups=radius.dev,resources=dapriodaprhttproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=dapriodaprhttproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=dapriodaprhttproutes/finalizers,verbs=update
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
//+kubebuilder:rbac:groups=radius.dev,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=gateways/finalizers,verbs=update

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
		r.Recorder.Eventf(resource, "Normal", "Waiting", "Application %s does not exist", applicationName)
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
		r.Recorder.Event(resource, "Normal", "Rendered", "Resource has been processed successfully")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ResourceReconciler) FetchKubernetesResources(ctx context.Context, log logr.Logger, resource *radiusv1alpha3.Resource) ([]client.Object, error) {
	log.Info("fetching existing resources for resource")
	results := []client.Object{}

	for _, a := range r.WatchedTypes {
		err := r.Client.List(ctx, a.ObjectList, client.InNamespace(resource.Namespace), client.MatchingFields{CacheKeyController: resource.Kind + resource.Name})
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to retrieve %T", a.ObjectList))
			return nil, err
		}

		err = meta.EachListItem(a.ObjectList, func(obj runtime.Object) error {
			o := obj.(client.Object)
			results = append(results, o)
			return nil
		})
		if err != nil {
			log.Error(err, "failed to get types")
			return nil, err
		}
	}

	log.Info("found existing resource for resource", "count", len(results))
	return results, nil
}

func (r *ResourceReconciler) RenderResource(ctx context.Context, req ctrl.Request, log logr.Logger, application *radiusv1alpha3.Application, resource *radiusv1alpha3.Resource, applicationName string, resourceName string) (*renderers.RendererOutput, bool, error) {
	// If the application hasn't been defined yet, then just produce no output.
	if application == nil {
		r.Recorder.Eventf(resource, "Normal", "Waiting", "Resource is waiting for application: %s", applicationName)
		return nil, false, nil
	}

	w := &renderers.RendererResource{}
	err := converters.ConvertToRenderResource(resource, w)
	if err != nil {
		r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource could not be converted: %v", err)
		log.Error(err, "failed to convert resource")
		return nil, false, err
	}

	resourceType, err := r.Model.LookupResource(w.ResourceType)
	if err != nil {
		r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource type '%s' is not supported'", w.ResourceType)
		log.Error(err, "unsupported type for resource")
		return nil, false, err
	}

	references, err := resourceType.Renderer().GetDependencyIDs(ctx, *w)
	if err != nil {
		r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource could not get dependencies: %v", err)
		log.Error(err, "failed to render resource")
		return nil, false, err
	}

	runtimeOptions, err := r.GetRuntimeOptions(ctx)
	if err != nil {
		r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource could not get additional properties: %v", err)
		log.Error(err, "failed to render resource")
	}

	deps := map[string]renderers.RendererDependency{}
	for _, reference := range references {
		dependency, err := r.GetRenderDependency(ctx, req.Namespace, reference)
		if err != nil {
			err = fmt.Errorf("failed to fetch rendering dependency %q of resource %q: %w", reference, resource.Name, err)
			r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource could not get dependencies", err)
			log.Error(err, "failed to render resource")
			return nil, false, err
		}

		deps[reference.ID] = *dependency
	}

	output, err := resourceType.Renderer().Render(ctx, renderers.RenderOptions{Resource: *w, Dependencies: deps, Runtime: runtimeOptions})
	if err != nil {
		r.Recorder.Eventf(resource, "Warning", "Invalid", "Resource had errors during rendering: %v'", err)
		log.Error(err, "failed to render resources for resource")
		return nil, false, err
	}

	log.Info("rendered output resources", "count", len(output.Resources))
	return &output, true, nil
}

func (r *ResourceReconciler) GetRuntimeOptions(ctx context.Context) (renderers.RuntimeOptions, error) {
	options := renderers.RuntimeOptions{}
	// We require a gateway class to be present before creating a gateway
	// Look up the first gateway class in the cluster and use that for now
	var gateways gatewayv1alpha1.GatewayClassList
	err := r.Client.List(ctx, &gateways)
	if err != nil {
		// Ignore failures to list gateway classes
		return renderers.RuntimeOptions{}, nil
	}

	if len(gateways.Items) > 0 {
		gatewayClass := gateways.Items[0]
		options.Gateway = renderers.GatewayOptions{
			GatewayClass: gatewayClass.Name,
		}
	}

	return options, nil
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
		err := converters.SetComputedValues(&resource.Status, desired.ComputedValues)
		if err != nil {
			return err
		}
	}

	if desired.SecretValues != nil {
		err := converters.SetSecretValues(&resource.Status, desired.SecretValues)
		if err != nil {
			return err
		}
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

func (r *ResourceReconciler) GetRenderDependency(ctx context.Context, namespace string, id azresources.ResourceID) (*renderers.RendererDependency, error) {
	// Find the Kubernetes resource based on the resourceID.
	if len(id.Types) < 3 {
		return nil, fmt.Errorf("dependency %q is not a radius resource", id)
	}

	resourceType := id.Types[2]
	unst := &unstructured.Unstructured{}

	// TODO determine this correctly
	unst.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(resourceType.Type),
	})

	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      kubernetes.MakeResourceName(id.Types[1].Name, id.Types[2].Name),
	}, unst)
	if err != nil {
		// TODO make this wait without an error?
		return nil, err
	}

	k8sResource := &radiusv1alpha3.Resource{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unst.Object, k8sResource)
	if err != nil {
		return nil, err
	}

	// The 'Definition' we provide to a dependency is actually the 'properties' node
	// of the ARM resource. Since 'spec.Template' stores the whole ARM resource we need
	// to drill down into 'properties'.
	body := map[string]interface{}{}
	err = json.Unmarshal(k8sResource.Spec.Template.Raw, &body)
	if err != nil {
		return nil, err
	}

	properties := map[string]interface{}{}
	obj, ok := body["properties"]
	if ok {
		// If properties is present it should be an object. It's not required in all cases.
		properties, ok = obj.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected %q to be a JSON object", "properties")
		}
	}

	outputResources := map[string]resourcemodel.ResourceIdentity{}
	for localID, outputResource := range k8sResource.Status.Resources {
		outputResources[localID] = resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Kind:       outputResource.Kind,
				APIVersion: outputResource.APIVersion,
				Name:       outputResource.Name,
				Namespace:  outputResource.Namespace,
			},
		}
	}

	// The 'ComputedValues' we provide to the dependency are a combination of the computed values
	// we store in status, and secrets we store separately.
	values := map[string]interface{}{}

	computedValues, err := converters.GetComputedValues(k8sResource.Status)
	if err != nil {
		return nil, err
	}

	for k, v := range computedValues {
		values[k] = v.Value
	}

	// The 'SecretValues' we store as part of the resource status (from render output) are references
	// to secrets, we need to fetch the values and pass them to the renderer.
	secretValues, err := converters.GetSecretValues(k8sResource.Status)
	if err != nil {
		return nil, err
	}

	secretClient := converters.SecretClient{Client: r.Client}
	for k, v := range secretValues {
		value, err := secretClient.LookupSecretValue(ctx, k8sResource.Status, v)
		if err != nil {
			return nil, err
		}

		values[k] = value
	}

	return &renderers.RendererDependency{
		ComputedValues:  values,
		ResourceID:      id,
		Definition:      properties,
		OutputResources: outputResources,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Index resources by application
	err := mgr.GetFieldIndexer().IndexField(context.Background(), r.ObjectType, CacheKeySpecApplication, extractApplicationKey)
	if err != nil {
		return err
	}

	cache := mgr.GetClient()
	applicationSource := &source.Kind{Type: &radiusv1alpha3.Application{}}
	applicationHandler := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []ctrl.Request {
		// Queue notification on each resource when the application changes.
		application := obj.(*radiusv1alpha3.Application)
		err := cache.List(context.Background(), r.ObjectList, client.InNamespace(application.Namespace), client.MatchingFields{CacheKeySpecApplication: application.Name})
		if err != nil {
			mgr.GetLogger().Error(err, "failed to list resources")
			return nil
		}

		requests := []ctrl.Request{}
		err = meta.EachListItem(r.ObjectList, func(obj runtime.Object) error {
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

	c := ctrl.NewControllerManagedBy(mgr).
		For(r.ObjectType).
		Watches(applicationSource, applicationHandler)
	for _, obj := range r.WatchedTypes {
		c = c.Owns(obj.Object)
	}

	return c.Complete(r)
}

func extractApplicationKey(obj client.Object) []string {
	return []string{obj.GetAnnotations()[kubernetes.LabelRadiusApplication]}
}
