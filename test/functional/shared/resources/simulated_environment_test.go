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

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Deployment_SimulatedEnv(t *testing.T) {
	template := "testdata/corerp-resources-simulatedenv.bicep"
	name := "corerp-resources-simulatedenv"
	appNamespace := "default-corerp-resources-simulatedenv"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-gtwy-gtwy-simulatedenv",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-rte-simulatedenv",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-ctnr-simulatedenv",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-rte-simulatedenv",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-ctnr-simulatedenv",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get pods in app namespace
				label := fmt.Sprintf("radius.dev/application=%s", name)
				pods, err := ct.Options.K8sClient.CoreV1().Pods(appNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: label,
				})
				require.NoError(t, err)
				// Verify no actual pods are deployed
				require.Equal(t, 0, len(pods.Items))
			},
		},
	})

	test.Test(t)
}
