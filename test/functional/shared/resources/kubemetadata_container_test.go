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

package resource_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_KubeMetadataContainer(t *testing.T) {
	template := "testdata/corerp-resources-kubemetadata-container.bicep"
	name := "corerp-kmd-app"
	appNamespace := "corerp-kmd-ns-corerp-kmd-app"

	expectedAnnotations := map[string]string{
		"user.cntr.ann.1": "user.cntr.ann.val.1",
		"user.cntr.ann.2": "user.cntr.ann.val.2",
	}

	expectedLabels := map[string]string{
		"user.cntr.lbl.1": "user.cntr.lbl.val.1",
		"user.cntr.lbl.2": "user.cntr.lbl.val.2",
	}

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-kmd-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-kmd-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "corerp-kmd-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				// Verify pod labels and annotations
				label := fmt.Sprintf("radius.dev/application=%s", name)
				pods, err := test.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)
				t.Logf("validated number of pods: %d", len(pods.Items))
				pod := pods.Items[0]
				require.True(t, functional.IsMapSubSet(expectedAnnotations, pod.Annotations))
				require.True(t, functional.IsMapSubSet(expectedLabels, pod.Labels))

				// Verify deployment labels and annotations
				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(context.Background(), metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, deployments.Items, 1)
				deployment := deployments.Items[0]
				require.True(t, functional.IsMapSubSet(expectedAnnotations, deployment.Annotations))
				require.True(t, functional.IsMapSubSet(expectedLabels, deployment.Labels))
			},
		},
	})

	test.Test(t)
}
