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

package reconciler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

func Test_markAndClearOwnedResourcesDeploying(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, radappiov1alpha3.AddToScheme(scheme))

	dt := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "app.bicep", Namespace: "ns", UID: types.UID("dt-uid"), Generation: 3},
	}
	ownedBy := func(name string, uid types.UID) *radappiov1alpha3.DeploymentResource {
		return &radappiov1alpha3.DeploymentResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "radapp.io/v1alpha3", Kind: "DeploymentTemplate", Name: "app.bicep", UID: uid, Controller: ptr.To(true),
				}},
			},
		}
	}
	owned := ownedBy("vote", "dt-uid")
	other := ownedBy("other", "other-uid")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dt, owned, other).Build()

	annotation := func(name string) (string, bool) {
		got := &radappiov1alpha3.DeploymentResource{}
		require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: name}, got))
		v, ok := got.Annotations[DeploymentResourceDeployingAnnotation]
		return v, ok
	}

	require.NoError(t, markOwnedResourcesDeploying(context.Background(), c, dt))
	v, ok := annotation("vote")
	require.True(t, ok)
	require.Equal(t, "3", v)
	_, ok = annotation("other")
	require.False(t, ok)

	require.NoError(t, clearOwnedResourcesDeploying(context.Background(), c, dt))
	_, ok = annotation("vote")
	require.False(t, ok)
}
