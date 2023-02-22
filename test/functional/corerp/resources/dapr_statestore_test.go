// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-generic.bicep"
	name := "corerp-resources-dapr-statestore-generic"
	appNamespace := "default-corerp-resources-dapr-statestore-generic"

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
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}

func Test_DaprStateStoreTableStorage(t *testing.T) {
	t.SkipNow()

	template := "testdata/corerp-resources-dapr-statestore-tablestorage.bicep"
	name := "corerp-resources-dapr-statestore-tablestorage"

	if os.Getenv("TABLESTORAGE_RESOURCE_ID") == "" {
		t.Error("TABLESTORAGE_RESOURCE_ID environment variable must be set to run this test.")
	}
	tablestorageresourceid := "tablestorageresourceid=" + os.Getenv("TABLESTORAGE_RESOURCE_ID")
	appNamespace := "default-corerp-resources-dapr-statestore-tablestorage"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), tablestorageresourceid),
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
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}

func Test_DaprStateStore_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-recipe.bicep"
	name := "corerp-resources-dss-recipe"
	appNamespace := "corerp-resources-dss-recipe-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-environment-recipes-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-dss-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "dss-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dss-recipe",
						Type: validation.DaprStateStoresResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDDaprStateStoreAzureStorage,
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureStorageTableService,
							},
							{
								Provider: resourcemodel.ProviderAzure,
								LocalID:  rpv1.LocalIDAzureStorageTable,
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dss-recipe-app-ctnr"),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}
	test.Test(t)
}
