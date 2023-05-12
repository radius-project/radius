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
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ContainerVersioning(t *testing.T) {
	t.Skip()
	containerV1 := "testdata/containers/corerp-resources-friendly-container-version-1.bicep"
	containerV2 := "testdata/containers/corerp-resources-friendly-container-version-2.bicep"

	name := "corerp-resources-container-versioning"
	appNamespace := "default-corerp-resources-container-versioning"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(containerV1, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "friendly-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "friendly-ctnr"),
					},
				},
			},
			SkipResourceDeletion: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				label := fmt.Sprintf("radius.dev/application=%s", name)
				secrets, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, secrets.Items, 1)
			},
		},
		{
			Executor: step.NewDeployExecutor(containerV2, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "friendly-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "friendly-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				label := fmt.Sprintf("radius.dev/application=%s", name)
				secrets, err := test.Options.K8sClient.CoreV1().Secrets(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				require.Len(t, secrets.Items, 0)
			},
		},
	})

	test.Test(t)
}
