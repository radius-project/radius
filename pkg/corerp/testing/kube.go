// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const trackerAddResourceVersion = "999"

var (
	dep = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deployment",
			Namespace:       "ns1",
			ResourceVersion: trackerAddResourceVersion,
		},
	}
)

type KubeFakeClient struct {
	clientset *fakek8s.Clientset
}

func (c *KubeFakeClient) Fake() *k8stesting.Fake {
	return &c.clientset.Fake
}

func (c *KubeFakeClient) DynamicClient(initObjs ...k8sclient.Object) k8sclient.WithWatch {
	return fakeclient.NewClientBuilder().
		WithRuntimeObjects(dep).
		WithObjects(initObjs...).
		WithObjectTracker(c.clientset.Tracker()).
		Build()
}

func NewKubeFakeClient() *KubeFakeClient {
	return &KubeFakeClient{
		clientset: fakek8s.NewSimpleClientset(),
	}
}
