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

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"
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
