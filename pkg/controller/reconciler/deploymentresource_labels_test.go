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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

func Test_syncDisplayLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       string
		phrase   radappiov1alpha3.DeploymentResourcePhrase
		expected map[string]string
	}{
		{
			name:   "container ready",
			id:     "/planes/radius/local/resourceGroups/g/providers/Radius.Compute/containers/vote",
			phrase: radappiov1alpha3.DeploymentResourcePhraseReady,
			expected: map[string]string{
				DeploymentResourceTypeLabel:   "Radius.Compute.containers",
				DeploymentResourcePhraseLabel: "Ready",
			},
		},
		{
			name:   "unparseable id keeps phrase only",
			id:     "not-an-id",
			phrase: radappiov1alpha3.DeploymentResourcePhraseFailed,
			expected: map[string]string{
				DeploymentResourcePhraseLabel: "Failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			require.NoError(t, radappiov1alpha3.AddToScheme(scheme))

			dr := &radappiov1alpha3.DeploymentResource{
				ObjectMeta: metav1.ObjectMeta{Name: "dr", Namespace: "ns"},
				Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: tt.id},
				Status:     radappiov1alpha3.DeploymentResourceStatus{Phrase: tt.phrase},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dr).Build()

			require.NoError(t, syncDisplayLabels(context.Background(), c, dr))

			got := &radappiov1alpha3.DeploymentResource{}
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dr), got))
			require.Equal(t, tt.expected, got.Labels)
		})
	}
}
