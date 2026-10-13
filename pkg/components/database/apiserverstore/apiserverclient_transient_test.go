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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
			err := client.get(context.Background(), runtimeclient.ObjectKey{Namespace: "radius-test", Name: "x"}, &ucpv1alpha1.Resource{})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.calls, calls)
		})
	}
}
