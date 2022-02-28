// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	radiusv1alpha3 "github.com/project-radius/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/project-radius/radius/pkg/kubernetes/webhook"
	"github.com/project-radius/radius/pkg/model"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var DefaultResourceTypes = []struct {
	client.Object
	client.ObjectList
}{
	{&radiusv1alpha3.Container{}, &radiusv1alpha3.ContainerList{}},
	{&radiusv1alpha3.DaprIOInvokeHttpRoute{}, &radiusv1alpha3.DaprIOInvokeHttpRouteList{}},
	{&radiusv1alpha3.DaprIOPubSubTopic{}, &radiusv1alpha3.DaprIOPubSubTopicList{}},
	{&radiusv1alpha3.DaprIOStateStore{}, &radiusv1alpha3.DaprIOStateStoreList{}},
	{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
	{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
	{&radiusv1alpha3.MongoDatabase{}, &radiusv1alpha3.MongoDatabaseList{}},
	{&radiusv1alpha3.RabbitMQMessageQueue{}, &radiusv1alpha3.RabbitMQMessageQueueList{}},
	{&radiusv1alpha3.RedisCache{}, &radiusv1alpha3.RedisCacheList{}},
	{&radiusv1alpha3.Gateway{}, &radiusv1alpha3.GatewayList{}},
	{&radiusv1alpha3.MicrosoftComSQLDatabase{}, &radiusv1alpha3.MicrosoftComSQLDatabaseList{}},
	{&radiusv1alpha3.Extender{}, &radiusv1alpha3.ExtenderList{}},
}

var DefaultWatchTypes = map[string]struct {
	Object        client.Object
	ObjectList    client.ObjectList
	HealthHandler func(ctx context.Context, r *ResourceReconciler, a client.Object) (string, string)
}{
	resourcekinds.Service:             {&corev1.Service{}, &corev1.ServiceList{}, nil},
	resourcekinds.Deployment:          {&appsv1.Deployment{}, &appsv1.DeploymentList{}, GetHealthStateFromDeployment},
	resourcekinds.Secret:              {&corev1.Secret{}, &corev1.SecretList{}, nil},
	resourcekinds.StatefulSet:         {&appsv1.StatefulSet{}, &appsv1.StatefulSetList{}, nil},
	resourcekinds.Gateway:             {&gatewayv1alpha1.Gateway{}, &gatewayv1alpha1.GatewayList{}, nil},
	resourcekinds.KubernetesHTTPRoute: {&gatewayv1alpha1.HTTPRoute{}, &gatewayv1alpha1.HTTPRouteList{}, nil},
}

type Options struct {
	Manager       manager.Manager
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
	WatchTypes map[string]struct {
		Object        client.Object
		ObjectList    client.ObjectList
		HealthHandler func(ctx context.Context, r *ResourceReconciler, a client.Object) (string, string)
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

	return &RadiusController{
		application: application,
		resources:   resources,
		options:     options,
	}
}

type RadiusController struct {
	application *ApplicationReconciler
	resources   []*ResourceReconciler
	options     *Options
}

func (c *RadiusController) Name() string {
	return "RadiusController"
}

func (c *RadiusController) Run(ctx context.Context) error {
	mgr := c.options.Manager
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

			// Get GVR for corresponding resource.
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

	if !c.options.SkipWebhooks {
		if err = (&radiusv1alpha3.Application{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup Application webhook: %w", err)
		}

		if err = (&webhook.ResourceWebhook{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to setup Resource webhook: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check %w", err)

	}

	if err := c.options.Manager.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager %w", err)
	}
	return nil
}
