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
	"testing"

	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ApplicationAndEnvironment(t *testing.T) {
	template := "testdata/corerp-resources-app-env.bicep"
	name := "corerp-resources-app-env"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-app-env-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-app-env-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			// Application and Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				expectedNS := []string{
					"corerp-resources-app-env",
					"corerp-resources-app-env-env-corerp-resources-app-env-app",
				}
				for _, ns := range expectedNS {
					_, err := test.Options.K8sClient.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
					require.NoErrorf(t, err, "%s must be created", ns)
				}
			},
		},
	})

	test.Test(t)
}
