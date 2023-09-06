/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"context"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	fakedynamic "k8s.io/client-go/dynamic/fake"
)

func TestCheckHTTPProxyStatus_ValidStatus(t *testing.T) {

	httpProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				kubernetes.LabelName:      "example.com",
			},
		},
		Status: contourv1.HTTPProxyStatus{
			CurrentStatus: HTTPProxyStatusValid,
		},
	}
	// create fake dynamic clientset
	s := runtime.NewScheme()
	err := contourv1.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := fakedynamic.NewSimpleDynamicClient(s, httpProxy)

	// create a fake dynamic informer factory with a mock HTTPProxy informer
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(fakeClient, 0, "default", nil)
	err = dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR).Informer().GetIndexer().Add(httpProxy)
	require.NoError(t, err, "Could not add test http proxy to informer cache")

	// create a mock object
	obj := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
		},
	}

	// create a channel for the done signal
	doneCh := make(chan error)

	httpProxyWaiter := &httpProxyWaiter{
		dynamicClientSet: fakeClient,
	}

	ctx := context.Background()
	dynamicInformerFactory.Start(ctx.Done())
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	// call the function with the fake clientset, informer factory, logger, object, and done channel
	go httpProxyWaiter.checkHTTPProxyStatus(context.Background(), dynamicInformerFactory, obj, doneCh)
	err = <-doneCh
	require.NoError(t, err)
}

func TestCheckHTTPProxyStatus_InvalidStatusForRootProxy(t *testing.T) {

	httpProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				kubernetes.LabelName:      "example.com",
			},
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "example.com",
			},
			Includes: []contourv1.Include{
				{
					Name:      "example.com",
					Namespace: "default",
				},
			},
		},
		Status: contourv1.HTTPProxyStatus{
			CurrentStatus: HTTPProxyStatusInvalid,
			Description:   "Failed to deploy HTTP proxy. see Errors for details",
			Conditions: []contourv1.DetailedCondition{
				{
					// specify Condition of type json
					Condition: metav1.Condition{
						Type:   HTTPProxyConditionValid,
						Status: contourv1.ConditionFalse,
					},
					Errors: []contourv1.SubCondition{
						{
							Type:    HTTPProxyConditionValid,
							Status:  contourv1.ConditionFalse,
							Reason:  "RouteNotDefined",
							Message: "HTTPProxy is invalid",
						},
					},
				},
			},
		},
	}
	// create fake dynamic clientset
	s := runtime.NewScheme()
	err := contourv1.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := fakedynamic.NewSimpleDynamicClient(s, httpProxy)

	// create a fake dynamic informer factory with a mock HTTPProxy informer
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(fakeClient, 0, "default", nil)
	err = dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR).Informer().GetIndexer().Add(httpProxy)
	require.NoError(t, err, "Could not add test http proxy to informer cache")

	// create a mock object
	obj := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
		},
	}

	// create a channel for the done signal
	doneCh := make(chan error)

	httpProxyWaiter := &httpProxyWaiter{
		dynamicClientSet: fakeClient,
	}

	ctx := context.Background()
	dynamicInformerFactory.Start(ctx.Done())
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	// call the function with the fake clientset, informer factory, logger, object, and done channel
	go httpProxyWaiter.checkHTTPProxyStatus(context.Background(), dynamicInformerFactory, obj, doneCh)
	err = <-doneCh
	require.EqualError(t, err, "Error - Type: Valid, Status: False, Reason: RouteNotDefined, Message: HTTPProxy is invalid\n")
}

func TestCheckHTTPProxyStatus_InvalidStatusForRouteProxy(t *testing.T) {
	httpProxyRoute := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				kubernetes.LabelName:      "example.com",
			},
		},
		Spec: contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Conditions: []contourv1.MatchCondition{
						{
							Prefix: "/",
						},
					},
					Services: []contourv1.Service{
						{
							Name: "test",
							Port: 80,
						},
					},
				},
			},
		},
		Status: contourv1.HTTPProxyStatus{
			CurrentStatus: HTTPProxyStatusInvalid,
			Description:   "Failed to deploy HTTP proxy. see Errors for details",
			Conditions: []contourv1.DetailedCondition{
				{
					// specify Condition of type json
					Condition: metav1.Condition{
						Type:   HTTPProxyConditionValid,
						Status: contourv1.ConditionFalse,
					},
					Errors: []contourv1.SubCondition{
						{
							Type:    HTTPProxyConditionValid,
							Status:  contourv1.ConditionFalse,
							Reason:  "orphaned",
							Message: "HTTPProxy is invalid",
						},
					},
				},
			},
		},
	}
	// create fake dynamic clientset
	s := runtime.NewScheme()
	err := contourv1.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := fakedynamic.NewSimpleDynamicClient(s, httpProxyRoute)

	// create a fake dynamic informer factory with a mock HTTPProxy informer
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(fakeClient, 0, "default", nil)
	err = dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR).Informer().GetIndexer().Add(httpProxyRoute)
	require.NoError(t, err, "Could not add test http proxy to informer cache")

	// create a mock object
	obj := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
		},
	}

	// create a channel for the done signal
	doneCh := make(chan error)

	httpProxyWaiter := &httpProxyWaiter{
		dynamicClientSet: fakeClient,
	}

	ctx := context.Background()
	dynamicInformerFactory.Start(ctx.Done())
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	// call the function with the fake clientset, informer factory, logger, object, and done channel
	go httpProxyWaiter.checkHTTPProxyStatus(context.Background(), dynamicInformerFactory, obj, doneCh)
	err = <-doneCh
	require.NoError(t, err)
}

func TestCheckHTTPProxyStatus_WrongSelector(t *testing.T) {

	httpProxy := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "abcd.com",
			Labels: map[string]string{
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				kubernetes.LabelName:      "abcd.com",
			},
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: "abcd.com",
			},
			Includes: []contourv1.Include{
				{
					Name:      "abcd.com",
					Namespace: "default",
				},
			},
		},
		Status: contourv1.HTTPProxyStatus{
			CurrentStatus: HTTPProxyStatusInvalid,
			Description:   "Failed to deploy HTTP proxy. see Errors for details",
			Conditions: []contourv1.DetailedCondition{
				{
					// specify Condition of type json
					Condition: metav1.Condition{
						Type:   HTTPProxyConditionValid,
						Status: contourv1.ConditionFalse,
					},
					Errors: []contourv1.SubCondition{
						{
							Type:    HTTPProxyConditionValid,
							Status:  contourv1.ConditionFalse,
							Reason:  "RouteNotDefined",
							Message: "HTTPProxy is invalid",
						},
					},
				},
			},
		},
	}

	// create fake dynamic clientset
	s := runtime.NewScheme()
	err := contourv1.AddToScheme(s)
	require.NoError(t, err)
	fakeClient := fakedynamic.NewSimpleDynamicClient(s, httpProxy)

	// create a fake dynamic informer factory with a mock HTTPProxy informer
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(fakeClient, 0, "default", nil)
	err = dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR).Informer().GetIndexer().Add(httpProxy)
	require.NoError(t, err, "Could not add test http proxy to informer cache")

	// create a mock object
	obj := &contourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "example.com",
		},
	}

	// create a channel for the done signal
	doneCh := make(chan error)

	httpProxyWaiter := &httpProxyWaiter{
		dynamicClientSet: fakeClient,
	}

	ctx := context.Background()
	dynamicInformerFactory.Start(ctx.Done())
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	// call the function with the fake clientset, informer factory, logger, object, and done channel
	status := httpProxyWaiter.checkHTTPProxyStatus(context.Background(), dynamicInformerFactory, obj, doneCh)
	require.False(t, status)
}
