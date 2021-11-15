// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"

	model "github.com/Azure/radius/pkg/model/typesv1alpha3"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	bicepcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/bicep"
	"github.com/Azure/radius/pkg/kubernetes/webhook"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var DefaultResourceTypes = []struct {
	client.Object
	client.ObjectList
}{
	{&radiusv1alpha3.ContainerComponent{}, &radiusv1alpha3.ContainerComponentList{}},
	{&radiusv1alpha3.DaprIODaprHttpRoute{}, &radiusv1alpha3.DaprIODaprHttpRouteList{}},
	{&radiusv1alpha3.DaprIOPubSubTopicComponent{}, &radiusv1alpha3.DaprIOPubSubTopicComponentList{}},
	{&radiusv1alpha3.DaprIOStateStoreComponent{}, &radiusv1alpha3.DaprIOStateStoreComponentList{}},
	{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
	{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
	{&radiusv1alpha3.MongoDBComponent{}, &radiusv1alpha3.MongoDBComponentList{}},
	{&radiusv1alpha3.RabbitMQComponent{}, &radiusv1alpha3.RabbitMQComponentList{}},
	{&radiusv1alpha3.RedisComponent{}, &radiusv1alpha3.RedisComponentList{}},
	{&radiusv1alpha3.Gateway{}, &radiusv1alpha3.GatewayList{}},
}

var DefaultWatchTypes = []struct {
	client.Object
	client.ObjectList
}{
	{&corev1.Service{}, &corev1.ServiceList{}},
	{&appsv1.Deployment{}, &appsv1.DeploymentList{}},
	{&corev1.Secret{}, &corev1.SecretList{}},
	{&appsv1.StatefulSet{}, &appsv1.StatefulSetList{}},
	{&gatewayv1alpha1.Gateway{}, &gatewayv1alpha1.GatewayList{}},
	{&gatewayv1alpha1.HTTPRoute{}, &gatewayv1alpha1.HTTPRouteList{}},
}

type Options struct {
	AppModel      model.ApplicationModel
	Client        client.Client
	Dynamic       dynamic.Interface
	Recorder      record.EventRecorder
	Scheme        *runtime.Scheme
	Log           logr.Logger
	RestConfig    *rest.Config
	RestMapper    meta.RESTMapper
	ResourceTypes []struct {
		client.Object
		client.ObjectList
	}
	WatchTypes []struct {
		client.Object
		client.ObjectList
	}
	SkipWebhooks bool
}

func NewRadiusController(options *Options) *RadiusController {
	application := &ApplicationReconciler{
		Client: options.Client,
		Scheme: options.Scheme,
		Log:    options.Log.WithName("controllers").WithName("Application"),
	}

	resources := []*ResourceReconciler{}
	for _, resourceType := range options.ResourceTypes {
		resource := &ResourceReconciler{
			Model:        options.AppModel,
			Client:       options.Client,
			Dynamic:      options.Dynamic,
			Scheme:       options.Scheme,
			Recorder:     options.Recorder,
			ObjectType:   resourceType.Object,
			ObjectList:   resourceType.ObjectList,
			Log:          ctrl.Log.WithName("controllers").WithName(resourceType.GetName()),
			WatchedTypes: options.WatchTypes,
		}
		resources = append(resources, resource)
	}

	template := &bicepcontroller.DeploymentTemplateReconciler{
		Client:        options.Client,
		DynamicClient: options.Dynamic,
		Scheme:        options.Scheme,
		RESTMapper:    options.RestMapper,
		Log:           options.Log.WithName("controllers").WithName("DeploymentTemplate"),
		Recorder:      options.Recorder,
	}

	return &RadiusController{
		application: application,
		resources:   resources,
		template:    template,
		options:     options,
	}
}

type RadiusController struct {
	application *ApplicationReconciler
	resources   []*ResourceReconciler
	template    *bicepcontroller.DeploymentTemplateReconciler
	options     *Options
}

func (c *RadiusController) SetupWithManager(mgr ctrl.Manager) error {
	err := c.application.SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup Application controller: %w", err)
	}

	// We create some indexes for watched types - this is done once because
	// we create a reconciler per-resource-type right now.

	// Index watched types by the owner (any resource besides application)
	for _, r := range c.options.WatchTypes {
		err = mgr.GetFieldIndexer().IndexField(context.Background(), r.Object, CacheKeyController, extractOwnerKey)
		if err != nil {
			return fmt.Errorf("failed to register index for %s: %w", "Deployment", err)
		}
	}

	for _, resource := range c.resources {
		gvks, _, err := c.options.Scheme.ObjectKinds(resource.ObjectType)
		if err != nil {
			return fmt.Errorf("unable to get GVK for resource type: %T: %w", resource.ObjectType, err)
		}

		for _, gvk := range gvks {
			if gvk.GroupVersion() != radiusv1alpha3.GroupVersion {
				continue
			}

			// Get GVR for corresponding component.
			gvr, err := c.options.RestMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				return fmt.Errorf("unable to get GVR for resource Kind: %s: %w", gvk.Kind, err)
			}

			resource.GVR = gvr.Resource
			err = resource.SetupWithManager(mgr)
			if err != nil {
				return fmt.Errorf("failed to setup Resource controller for %T: %w", resource.ObjectType, err)
			}
		}
	}

	err = c.template.SetupWithManager(mgr, c.options.ResourceTypes)
	if err != nil {
		return err
	}

	if !c.options.SkipWebhooks {
		if err = (&radiusv1alpha3.Application{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup Application webhook: %w", err)
		}

		if err = (&webhook.ResourceWebhook{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup Resource webhook: %w", err)
		}

		if err = (&bicepv1alpha3.DeploymentTemplate{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup DeploymentTemplate webhook: %w", err)
		}
	}

	return nil
}
