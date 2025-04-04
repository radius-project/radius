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

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Opt-out case for manual resource provisioning
func Test_MongoDB_Manual(t *testing.T) {
	template := "testdata/datastoresrp-rs-mongodb-manual.bicep"
	name := "dsrp-resources-mongodb-manual"
	appNamespace := "default-cdsrp-resources-mongodb-manual"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mdb-us-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mdb-us-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "mdb-us-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDB_Recipe(t *testing.T) {
	template := "testdata/datastoresrp-resources-mongodb-recipe.bicep"
	name := "dsrp-resources-mongodb-recipe"
	appNamespace := "dsrp-resources-mongodb-recipe-app"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dsrp-resources-mongodb-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dsrp-resources-mongodb-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongodb-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongodb-app-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}

// This test verifies deployment of shared environment scoped resource using 'existing' keyword.
// It has 2 steps:
// 1. Deploy the environment and mongodb resource to the environment namespace.
// 2. Deploy an app that uses the existing mongodb resource using the 'existing' keyword.
func Test_MongoDB_EnvScoped_ExistingResource(t *testing.T) {
	envTemplate := "testdata/datastoresrp-resources-mongodb-recipe-and-env.bicep"
	existingTemplate := "testdata/datastoresrp-resources-mongodb-existing-env-scoped-resource.bicep"
	name := "mongodb-recipe-and-env"
	appNamespace := "mongodb-recipe-existing-app"
	appName := "mongodb-recipe-existing"
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(envTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "existing-mongodb",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
		{
			Executor: step.NewDeployExecutor(existingTemplate, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
						App:  appName,
					},
					{
						Name: "mongo-ctnr-exst",
						Type: validation.ContainersResource,
						App:  appName,
					},
					{
						Name: "existing-mongodb",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mongo-ctnr-exst").ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				_, err := ct.Options.K8sClient.CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
				require.NoError(t, err)
			},
		},
	})
	test.Test(t)
}
