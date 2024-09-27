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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_ScopedResourcesManual(t *testing.T) {
	template := "testdata/daprrp-scoped-resources-manual.bicep"
	name := "dapr-scopes-manual"
	appNamespace := fmt.Sprintf("default-%s", name)
	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            v1.CodeOperationCanceled,
				MessageContains: "Operation (APPLICATIONS.CORE/CONTAINERS|PUT) has timed out because it was processing longer than",
				TargetEndsWith:  fmt.Sprintf("%s-ctnr-ko", name),
			},
		},
	})
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(
				template,
				validate,
				testutil.GetMagpieImage(),
				fmt.Sprintf("namespace=%s", appNamespace),
				fmt.Sprintf("baseName=%s", name),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: fmt.Sprintf("%s-ctnr-ok", name),
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: fmt.Sprintf("%s-ctnr-ko", name),
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: fmt.Sprintf("%s-sts", name),
						Type: validation.DaprStateStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-ctnr-ok", name)),
						// Deployed as supporting resources using Kubernetes Bicep extensibility.
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),
						validation.NewK8sServiceForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),

						validation.NewDaprComponent(name, fmt.Sprintf("%s-sts", name)).
							ValidateLabels(false),
					},
				},
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureDapr}

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		verifyDaprComponentsDeleted(ctx, t, test, "Applications.Dapr/stateStores", fmt.Sprintf("%s-sts", name), appNamespace)

	}

	test.Test(t)
}
