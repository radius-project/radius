// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Model interface {
	GetWatchedTypes() []client.Object
	GetReconciledTypes() []ReconcilableType
}

type coolmodel struct {
	WatchedTypes    []client.Object
	ReconciledTypes []ReconcilableType
}

func NewModel(watchedTypes []client.Object, reconciledTypes []ReconcilableType) Model {
	return &coolmodel{
		WatchedTypes:    watchedTypes,
		ReconciledTypes: reconciledTypes,
	}
}

func NewKubernetesModel() Model {
	return NewModel(
		[]client.Object{
			&corev1.Service{},
			&appsv1.Deployment{},
		},
		[]ReconcilableType{
			{&radiusv1alpha3.ContainerComponent{}, &radiusv1alpha3.ContainerComponentList{}},
			{&radiusv1alpha3.DaprIODaprHttpRoute{}, &radiusv1alpha3.DaprIODaprHttpRouteList{}},
			{&radiusv1alpha3.DaprIOPubSubTopicComponent{}, &radiusv1alpha3.DaprIOPubSubTopicComponentList{}},
			{&radiusv1alpha3.DaprIOStateStoreComponent{}, &radiusv1alpha3.DaprIOStateStoreComponentList{}},
			{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
			{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
			{&radiusv1alpha3.MongoDBComponent{}, &radiusv1alpha3.MongoDBComponentList{}},
			{&radiusv1alpha3.RabbitMQComponent{}, &radiusv1alpha3.RabbitMQComponentList{}},
			{&radiusv1alpha3.RedisComponent{}, &radiusv1alpha3.RedisComponentList{}},
		},
	)
}

func NewLocalModel() Model {
	return NewModel(
		[]client.Object{},
		[]ReconcilableType{
			{&radiusv1alpha3.ContainerComponent{}, &radiusv1alpha3.ContainerComponentList{}},
			{&radiusv1alpha3.DaprIODaprHttpRoute{}, &radiusv1alpha3.DaprIODaprHttpRouteList{}},
			{&radiusv1alpha3.DaprIOPubSubTopicComponent{}, &radiusv1alpha3.DaprIOPubSubTopicComponentList{}},
			{&radiusv1alpha3.DaprIOStateStoreComponent{}, &radiusv1alpha3.DaprIOStateStoreComponentList{}},
			{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
			{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
			{&radiusv1alpha3.MongoDBComponent{}, &radiusv1alpha3.MongoDBComponentList{}},
			{&radiusv1alpha3.RabbitMQComponent{}, &radiusv1alpha3.RabbitMQComponentList{}},
			{&radiusv1alpha3.RedisComponent{}, &radiusv1alpha3.RedisComponentList{}},
		},
	)
}

func (m *coolmodel) GetWatchedTypes() []client.Object {
	return m.WatchedTypes
}

func (m *coolmodel) GetReconciledTypes() []ReconcilableType {
	return m.ReconciledTypes
}

type ReconcilableType struct {
	Object     client.Object
	ObjectList client.ObjectList
}
