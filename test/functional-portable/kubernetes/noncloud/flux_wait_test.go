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

package kubernetes_noncloud_test

import (
	"context"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_waitForDeploymentTemplateToBeReadyWithGenerationTimeout(t *testing.T) {
	t.Run("returns ready deployment template", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, radappiov1alpha3.AddToScheme(scheme))

		name := types.NamespacedName{Name: "flux-complex.bicep", Namespace: "flux-complex"}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&radappiov1alpha3.DeploymentTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Status: radappiov1alpha3.DeploymentTemplateStatus{
				Phrase:             radappiov1alpha3.DeploymentTemplatePhraseReady,
				ObservedGeneration: 2,
			},
		}).Build()

		deploymentTemplate, err := waitForDeploymentTemplateToBeReadyWithGenerationTimeout(t, context.Background(), name, 2, client, 50*time.Millisecond, time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, deploymentTemplate)
		require.Equal(t, int64(2), deploymentTemplate.Status.ObservedGeneration)
	})

	t.Run("times out while deployment template is still updating", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, radappiov1alpha3.AddToScheme(scheme))

		name := types.NamespacedName{Name: "flux-complex.bicep", Namespace: "flux-complex"}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&radappiov1alpha3.DeploymentTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Status: radappiov1alpha3.DeploymentTemplateStatus{
				Phrase:             radappiov1alpha3.DeploymentTemplatePhraseUpdating,
				ObservedGeneration: 1,
			},
		}).Build()

		deploymentTemplate, err := waitForDeploymentTemplateToBeReadyWithGenerationTimeout(t, context.Background(), name, 2, client, 10*time.Millisecond, time.Millisecond)
		require.Error(t, err)
		require.NotNil(t, deploymentTemplate)
		require.Contains(t, err.Error(), "not ready")
		require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, deploymentTemplate.Status.Phrase)
	})
}
