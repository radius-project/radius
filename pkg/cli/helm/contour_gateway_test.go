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

package helm

import (
	"context"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestReconcileDefaultContourGatewayCreatesResources(t *testing.T) {
	t.Parallel()

	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())

	err := reconcileDefaultContourGateway(context.Background(), client)
	require.NoError(t, err)

	gatewayClass, err := client.Resource(gatewayClassGVR).Get(context.Background(), ContourGatewayClassName, metav1.GetOptions{})
	require.NoError(t, err)
	controllerName, _, _ := unstructured.NestedString(gatewayClass.Object, "spec", "controllerName")
	require.Equal(t, ContourGatewayControllerName, controllerName)
	require.True(t, isRadiusManaged(gatewayClass))

	gateway, err := client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace).Get(context.Background(), DefaultContourGatewayName, metav1.GetOptions{})
	require.NoError(t, err)
	gatewayClassName, _, _ := unstructured.NestedString(gateway.Object, "spec", "gatewayClassName")
	require.Equal(t, ContourGatewayClassName, gatewayClassName)
	require.True(t, isRadiusManaged(gateway))
	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	require.Len(t, listeners, 2)
	require.Equal(t, map[string]any{
		"name":     "http",
		"protocol": "HTTP",
		"port":     int64(80),
		"allowedRoutes": map[string]any{
			"namespaces": map[string]any{
				"from": "All",
			},
		},
	}, listeners[0])
	require.Equal(t, map[string]any{
		"name":     "https",
		"protocol": "HTTPS",
		"port":     int64(443),
		"allowedRoutes": map[string]any{
			"namespaces": map[string]any{
				"from": "All",
			},
		},
	}, listeners[1])
}

func TestWaitForDefaultContourGatewayRetriesGatewayAPINotFound(t *testing.T) {
	t.Parallel()

	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme())
	createGatewayClassAttempts := 0
	client.PrependReactor("create", "gatewayclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createGatewayClassAttempts++
		if createGatewayClassAttempts == 1 {
			return true, nil, apierrors.NewNotFound(gatewayClassGVR.GroupResource(), ContourGatewayClassName)
		}

		return false, nil, nil
	})

	err := waitForDefaultContourGateway(context.Background(), client, time.Millisecond, time.Second)
	require.NoError(t, err)
	require.Equal(t, 2, createGatewayClassAttempts)

	_, err = client.Resource(gatewayClassGVR).Get(context.Background(), ContourGatewayClassName, metav1.GetOptions{})
	require.NoError(t, err)
	_, err = client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace).Get(context.Background(), DefaultContourGatewayName, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestReconcileDefaultContourGatewayAllowsExistingMatchingGatewayClass(t *testing.T) {
	t.Parallel()

	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name": ContourGatewayClassName,
			},
			"spec": map[string]any{
				"controllerName": ContourGatewayControllerName,
			},
		},
	})
	err := reconcileDefaultContourGateway(context.Background(), client)
	require.NoError(t, err)
}

func TestReconcileDefaultContourGatewayPreservesExistingGatewayMetadata(t *testing.T) {
	t.Parallel()

	existingGateway := newContourGateway()
	existingGateway.SetAnnotations(map[string]string{
		"example.com/annotation": "preserve",
	})
	existingGateway.SetFinalizers([]string{"example.com/finalizer"})
	existingGateway.SetLabels(map[string]string{
		kubernetes.LabelManagedBy: radiusManagedValue,
		kubernetes.LabelPartOf:    radiusManagedValue,
		"example.com/label":       "preserve",
	})
	require.NoError(t, unstructured.SetNestedSlice(existingGateway.Object, []any{
		map[string]any{
			"name":     "old",
			"protocol": "HTTP",
			"port":     int64(8080),
		},
	}, "spec", "listeners"))

	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme(), newContourGatewayClass())
	_, err := client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace).Create(context.Background(), existingGateway, metav1.CreateOptions{})
	require.NoError(t, err)

	err = reconcileDefaultContourGateway(context.Background(), client)
	require.NoError(t, err)

	gateway, err := client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace).Get(context.Background(), DefaultContourGatewayName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "preserve", gateway.GetAnnotations()["example.com/annotation"])
	require.Equal(t, []string{"example.com/finalizer"}, gateway.GetFinalizers())
	require.Equal(t, "preserve", gateway.GetLabels()["example.com/label"])
	require.True(t, isRadiusManaged(gateway))

	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	require.Len(t, listeners, 2)
	require.Equal(t, "http", listeners[0].(map[string]any)["name"])
	require.Equal(t, "https", listeners[1].(map[string]any)["name"])
}

func TestReconcileDefaultContourGatewayRejectsConflictingGatewayClass(t *testing.T) {
	t.Parallel()

	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name": ContourGatewayClassName,
			},
			"spec": map[string]any{
				"controllerName": "example.com/other-controller",
			},
		},
	})
	err := reconcileDefaultContourGateway(context.Background(), client)
	require.ErrorContains(t, err, "already exists with controllerName")
}

func TestDeleteDefaultContourGatewayResourcesOnlyDeletesManagedResources(t *testing.T) {
	t.Parallel()

	unmanagedGatewayClass := newContourGatewayClass()
	unmanagedGatewayClass.SetLabels(nil)
	managedGateway := newContourGateway()
	client := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme(), unmanagedGatewayClass, managedGateway)

	err := deleteDefaultContourGatewayResources(context.Background(), client)
	require.NoError(t, err)

	_, err = client.Resource(gatewayClassGVR).Get(context.Background(), ContourGatewayClassName, metav1.GetOptions{})
	require.NoError(t, err)
	_, err = client.Resource(gatewayGVR).Namespace(DefaultContourGatewayNamespace).Get(context.Background(), DefaultContourGatewayName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}
