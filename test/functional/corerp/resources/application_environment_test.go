// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ApplicationAndEnvironment(t *testing.T) {
	template := "testdata/corerp-resources-app-env.bicep"
	name := "corerp-resources-app-env"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				appNS := "corerp-resources-application-app"
				_, err := test.Options.K8sClient.CoreV1().Namespaces().Get(ctx, appNS, metav1.GetOptions{})
				require.NoErrorf(t, err, "%s must be created", appNS)
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
