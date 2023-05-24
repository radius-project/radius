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

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprSecretStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-secretstore-generic.bicep"
	name := "corerp-resources-dapr-secretstore-generic"
	appNamespace := "default-corerp-resources-dapr-secretstore-generic"

	test := corerp.NewCoreRPTest(t, appNamespace, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						Name: "gnrc-scs",
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
	}, corerp.K8sSecretResource(appNamespace, "mysecret", "", "fakekey", []byte("fakevalue")))
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}
