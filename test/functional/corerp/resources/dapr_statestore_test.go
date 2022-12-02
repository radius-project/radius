// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-generic.bicep"
	name := "corerp-resources-dapr-statestore-generic"
	appNamespace := "default-corerp-resources-dapr-statestore-generic"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-statestore-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gnrc-sts-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "gnrc-sts",
						Type: validation.DaprStateStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "gnrc-sts-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_DaprStateStoreTableStorage(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-tablestorage.bicep"
	name := "corerp-resources-dapr-statestore-tablestorage"
	appNamespace := "default-corerp-resources-dapr-statestore-tablestorage"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-statestore-tablestorage",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ts-sts-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "ts-sts",
						Type: validation.DaprStateStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ts-sts-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
