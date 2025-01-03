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

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_Binding_Manual_Secret(t *testing.T) {
	template := "testdata/daprrp-resources-binding-manual-secret.bicep"
	name := "dbd-manual-secret"
	appNamespace := fmt.Sprintf("default-%s", name)
	redisPassword := "Password1234!"
	secretName := "redisauth"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				testutil.GetMagpieImage(),
				fmt.Sprintf("namespace=%s", appNamespace),
				fmt.Sprintf("baseName=%s", name),
				fmt.Sprintf("redisPassword=%s", redisPassword),
				fmt.Sprintf("secretName=%s", secretName),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: fmt.Sprintf("%s-ctnr", name),
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: fmt.Sprintf("%s-dbd", name),
						Type: validation.DaprBindingsResource,
						App:  name,
					},
					{
						Name: fmt.Sprintf("%s-scs", name),
						Type: validation.DaprSecretStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-ctnr", name)),

						// Deployed as supporting resources using Kubernetes Bicep extensibility.
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),
						validation.NewK8sServiceForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),

						validation.NewDaprComponent(name, fmt.Sprintf("%s-dbd", name)).
							ValidateLabels(false),
						validation.NewDaprComponent(name, fmt.Sprintf("%s-scs", name)).
							ValidateLabels(false),
					},
				},
			},
		},
	}, rp.K8sSecretResource(appNamespace, secretName, "", "password", redisPassword))

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureDapr}

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		verifyDaprComponentsDeleted(ctx, t, test, "Applications.Dapr/bindings", fmt.Sprintf("%s-dbd", name), appNamespace)
		verifyDaprComponentsDeleted(ctx, t, test, "Applications.Dapr/secretStores", fmt.Sprintf("%s-scs", name), appNamespace)

	}

	test.Test(t)
}

func Test_Binding_Manual(t *testing.T) {
	template := "testdata/daprrp-resources-binding-manual.bicep"
	name := "dbd-manual"
	appNamespace := fmt.Sprintf("default-%s", name)
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
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
						Name: fmt.Sprintf("%s-ctnr", name),
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: fmt.Sprintf("%s-dbd", name),
						Type: validation.DaprBindingsResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-ctnr", name)),

						// Deployed as supporting resources using Kubernetes Bicep extensibility.
						validation.NewK8sPodForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),
						validation.NewK8sServiceForResource(name, fmt.Sprintf("%s-redis", name)).
							ValidateLabels(false),

						validation.NewDaprComponent(name, fmt.Sprintf("%s-dbd", name)).
							ValidateLabels(false),
					},
				},
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureDapr}

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		verifyDaprComponentsDeleted(ctx, t, test, "Applications.Dapr/bindings", fmt.Sprintf("%s-dbd", name), appNamespace)
	}

	test.Test(t)
}
