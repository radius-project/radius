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

package apiserverstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/radius-project/radius/pkg/components/database"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
)

func Test_Get_RetriesTransientErrors(t *testing.T) {
	original := transientBackoff
	transientBackoff = wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3}
	t.Cleanup(func() { transientBackoff = original })

	unavailable := apierrors.NewServiceUnavailable("the server is currently unable to handle the request")

	tests := []struct {
		name     string
		failures int
		err      error
		wantErr  bool
		calls    int
	}{
		{"recovers after transient errors", 2, unavailable, false, 3},
		{"gives up after too many transient errors", 10, unavailable, true, 3},
		{"does not retry other errors", 10, apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("no")), true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			fakeClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					calls++
					if calls <= tt.failures {
						return tt.err
					}
					return nil
				},
			}).Build()

			client := NewAPIServerClient(fakeClient, "radius-test")
			err := client.get(t.Context(), runtimeclient.ObjectKey{Namespace: "radius-test", Name: "x"}, &ucpv1alpha1.Resource{})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.calls, calls)
		})
	}
}

func Test_Get_StopsRetryingWhenContextCancelled(t *testing.T) {
	original := transientBackoff
	transientBackoff = wait.Backoff{Duration: time.Hour, Factor: 1, Steps: 5}
	t.Cleanup(func() { transientBackoff = original })

	calls := 0
	fakeClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			calls++
			return apierrors.NewServiceUnavailable("unavailable")
		},
	}).Build()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := NewAPIServerClient(fakeClient, "radius-test")
	err := client.get(ctx, runtimeclient.ObjectKey{Namespace: "radius-test", Name: "x"}, &ucpv1alpha1.Resource{})
	require.True(t, apierrors.IsServiceUnavailable(err))
	require.Equal(t, 1, calls)
}

func Test_Operations_UseRetryingGet(t *testing.T) {
	original := transientBackoff
	transientBackoff = wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3}
	t.Cleanup(func() { transientBackoff = original })

	id := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

	// Fail the first read of each operation, then let the fake client answer.
	calls := 0
	scheme := runtime.NewScheme()
	require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			calls++
			if calls == 1 {
				return apierrors.NewServiceUnavailable("unavailable")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	client := NewAPIServerClient(fakeClient, "radius-test")

	_, err := client.Get(t.Context(), id)
	var notFound *database.ErrNotFound
	require.ErrorAs(t, err, &notFound)

	calls = 0
	err = client.Delete(t.Context(), id)
	require.ErrorAs(t, err, &notFound)

	calls = 0
	err = client.Save(t.Context(), &database.Object{ID: id, Data: map[string]any{}})
	require.NoError(t, err)
}

func Test_Query_RetriesTransientList(t *testing.T) {
	original := transientBackoff
	transientBackoff = wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3}
	t.Cleanup(func() { transientBackoff = original })

	scheme := runtime.NewScheme()
	require.NoError(t, ucpv1alpha1.AddToScheme(scheme))

	calls := 0
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c runtimeclient.WithWatch, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			calls++
			if calls == 1 {
				return apierrors.NewTooManyRequests("slow down", 1)
			}
			return c.List(ctx, list, opts...)
		},
	}).Build()

	client := NewAPIServerClient(fakeClient, "radius-test")
	result, err := client.Query(t.Context(), database.Query{
		RootScope:    "/planes/radius/local/resourceGroups/test-group",
		ResourceType: "Applications.Core/applications",
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.Equal(t, 2, calls)
}
