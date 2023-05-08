/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package handlers

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_CheckDaprResourceNameUniqueness_NotFound(t *testing.T) {
	client := k8sutil.NewFakeKubeClient(nil)

	err := CheckDaprResourceNameUniqueness(context.Background(), client, "test-component", "default", "test-resource", linkrp.DaprStateStoresResourceType)
	require.NoError(t, err)
}

func Test_CheckDaprResourceNameUniqueness_SameRadiusResource(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", linkrp.DaprStateStoresResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(context.Background(), client, "test-component", "default", "test-resource", linkrp.DaprStateStoresResourceType)
	require.NoError(t, err)
}

func Test_CheckDaprResourceNameUniqueness_NoLabels(t *testing.T) {
	existing := createUnstructuredComponent("test-component", "default", nil)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(context.Background(), client, "test-component", "default", "test-resource", linkrp.DaprStateStoresResourceType)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf(daprConflictFmt, "test-component"), err.Error())
}

func Test_CheckDaprResourceNameUniqueness_DifferentResourceNames(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "different-resource", linkrp.DaprStateStoresResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(context.Background(), client, "test-component", "default", "test-resource", linkrp.DaprStateStoresResourceType)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf(daprConflictFmt, "test-component"), err.Error())
}

func Test_CheckDaprResourceNameUniqueness_DifferentResourceTypes(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", linkrp.DaprPubSubBrokersResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(context.Background(), client, "test-component", "default", "test-resource", linkrp.DaprStateStoresResourceType)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf(daprConflictFmt, "test-component"), err.Error())
}

func createUnstructuredComponent(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "Component",
		Version: "dapr.io/v1alpha1",
	})
	u.SetNamespace(namespace)
	u.SetName(name)

	if labels == nil {
		return u
	}

	// This is (unfortunately) needed, because unstructured wants a map[string]string for labels. However
	// some of the fake clients want a map[string]any and WILL NOT work with map[string]string. So our API
	// returns map[string]any.
	copy := map[string]string{}
	for k, v := range labels {
		copy[k] = v.(string)
	}

	u.SetLabels(copy)
	return u
}
