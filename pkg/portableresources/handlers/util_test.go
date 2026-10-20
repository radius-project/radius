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
	"errors"
	"fmt"
	"testing"
	"time"

	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func Test_ApplyDaprComponent_ConcurrentNames(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	k8s := interceptor.NewClient(newDaprComponentClient(), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			ready <- struct{}{}
			select {
			case <-release:
				return c.Create(ctx, obj, opts...)
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	for _, resourceType := range []string{dapr_ctrl.DaprPubSubBrokersResourceType, dapr_ctrl.DaprSecretStoresResourceType} {
		go func() {
			labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", resourceType)
			component := createUnstructuredComponent("test-component", "default", labels)
			results <- ApplyDaprComponent(ctx, k8s, component, "test-resource", resourceType)
		}()
	}
	// Both requests must reach creation before either is allowed to claim the name.
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("both resources did not reach component creation")
		}
	}
	close(release)

	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		require.IsType(t, &processors.ValidationError{}, err)
		require.EqualError(t, err, fmt.Sprintf(daprConflictFmt, "test-component"))
	}
	require.Equal(t, 1, successes)
}

func Test_ApplyDaprComponent_Update(t *testing.T) {
	t.Parallel()
	k8s := newDaprComponentClient()
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	component := createUnstructuredComponent("test-component", "default", labels)
	component.Object["spec"] = map[string]any{"type": "state.redis", "version": "v1"}
	require.NoError(t, ApplyDaprComponent(t.Context(), k8s, component, "test-resource", dapr_ctrl.DaprStateStoresResourceType))

	update := createUnstructuredComponent("test-component", "default", labels)
	update.Object["spec"] = map[string]any{"type": "state.redis", "version": "v2"}
	require.NoError(t, ApplyDaprComponent(t.Context(), k8s, update, "test-resource", dapr_ctrl.DaprStateStoresResourceType))
	actual := createUnstructuredComponent("test-component", "default", nil)
	require.NoError(t, k8s.Get(t.Context(), client.ObjectKeyFromObject(actual), actual))
	require.Equal(t, update.Object["spec"], actual.Object["spec"])
}

func Test_ApplyDaprComponent_OwnershipChangedBeforeApply(t *testing.T) {
	t.Parallel()
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	k8s := interceptor.NewClient(newDaprComponentClient(existing), interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			replacement := existing.DeepCopy()
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), replacement); err != nil {
				return err
			}
			replacementLabels := replacement.GetLabels()
			replacementLabels[kubernetes.LabelRadiusResource] = "another-resource"
			replacement.SetLabels(replacementLabels)
			if err := c.Update(ctx, replacement); err != nil {
				return err
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	})
	component := createUnstructuredComponent("test-component", "default", labels)
	err := ApplyDaprComponent(t.Context(), k8s, component, "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	require.True(t, k8serrors.IsConflict(err), "expected an optimistic concurrency conflict, got %v", err)
	actual := createUnstructuredComponent("test-component", "default", nil)
	require.NoError(t, k8s.Get(t.Context(), client.ObjectKeyFromObject(actual), actual))
	require.Equal(t, "another-resource", actual.GetLabels()[kubernetes.LabelRadiusResource])
}

func Test_ApplyDaprComponent_Errors(t *testing.T) {
	for _, operation := range []string{"create", "get", "patch"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			expected := errors.New("Kubernetes API unavailable")
			hooks := interceptor.Funcs{}
			switch operation {
			case "create":
				hooks.Create = func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
					return expected
				}
			case "get":
				hooks.Get = func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return expected
				}
			case "patch":
				hooks.Patch = func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
					return expected
				}
			}
			labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
			existing := createUnstructuredComponent("test-component", "default", labels)
			k8s := interceptor.NewClient(newDaprComponentClient(existing), hooks)
			component := createUnstructuredComponent("test-component", "default", labels)
			err := ApplyDaprComponent(t.Context(), k8s, component, "test-resource", dapr_ctrl.DaprStateStoresResourceType)
			require.IsType(t, &processors.ResourceError{}, err)
			require.ErrorIs(t, err, expected)
		})
	}
}

func newDaprComponentClient(objects ...client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "dapr.io", Version: "v1alpha1", Kind: "Component"}, &unstructured.Unstructured{})
	return k8sutil.NewFakeKubeClient(scheme, objects...)
}

func Test_CheckDaprResourceNameUniqueness_NotFound(t *testing.T) {
	client := k8sutil.NewFakeKubeClient(nil)

	err := CheckDaprResourceNameUniqueness(t.Context(), client, "test-component", "default", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	require.NoError(t, err)
}

func Test_CheckDaprResourceNameUniqueness_SameRadiusResource(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(t.Context(), client, "test-component", "default", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	require.NoError(t, err)
}

func Test_CheckDaprResourceNameUniqueness_NoLabels(t *testing.T) {
	existing := createUnstructuredComponent("test-component", "default", nil)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(t.Context(), client, "test-component", "default", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf(daprConflictFmt, "test-component"), err.Error())
}

func Test_CheckDaprResourceNameUniqueness_DifferentResourceNames(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "different-resource", dapr_ctrl.DaprStateStoresResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(t.Context(), client, "test-component", "default", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf(daprConflictFmt, "test-component"), err.Error())
}

func Test_CheckDaprResourceNameUniqueness_DifferentResourceTypes(t *testing.T) {
	labels := kubernetes.MakeDescriptiveDaprLabels("test-app", "test-resource", dapr_ctrl.DaprPubSubBrokersResourceType)
	existing := createUnstructuredComponent("test-component", "default", labels)
	client := k8sutil.NewFakeKubeClient(nil, existing)

	err := CheckDaprResourceNameUniqueness(t.Context(), client, "test-component", "default", "test-resource", dapr_ctrl.DaprStateStoresResourceType)
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
