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
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_DaprSecretStore_Manual(t *testing.T) {
	template := "resources/testdata/daprrp-resources-secretstore-manual.bicep"
	name := "daprrp-rs-secretstore-manual"
	appNamespace := "default-daprrp-rs-secretstore-manual"

	test := shared.NewRPTest(t, appNamespace, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gnrc-scs-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "gnrc-scs-manual",
						Type: validation.DaprSecretStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "gnrc-scs-ctnr"),
					},
				},
			},
		},
	}, shared.K8sSecretResource(appNamespace, "mysecret", "", "fakekey", []byte("fakevalue")))
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}

	test.Test(t)
}

func Test_DaprSecretStore_Recipe(t *testing.T) {
	template := "resources/testdata/daprrp-resources-secretstore-recipe.bicep"
	name := "daprrp-rs-secretstore-recipe"
	appNamespace := "daprrp-rs-secretstore-recipe"

	test := shared.NewRPTest(t, appNamespace, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gnrc-scs-ctnr-recipe",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "gnrc-scs-recipe",
						Type: validation.DaprSecretStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "gnrc-scs-ctnr-recipe").ValidateLabels(false),
					},
				},
			},
		},
	}, shared.K8sSecretResource(appNamespace, "mysecret", "", "fakekey", []byte("fakevalue")))
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}

	test.Test(t)
}
