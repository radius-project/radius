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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_DaprSecretStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-secretstore-generic.bicep"
	name := "corerp-resources-dapr-secretstore-generic"
	appNamespace := "default-corerp-resources-dapr-secretstore-generic"

	requiredSecrets := map[string]map[string]string{}

	// TODO: remove requiredSecrets, but instead use the below initialResource approach.
	resources := []unstructured.Unstructured{
		{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name":      "mysecret",
					"namespace": appNamespace,
				},
				"data": map[string]any{
					"mysecret": []byte("mysecret"),
				},
			},
		},
	}

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
	}, requiredSecrets, resources...)

	test.Test(t)
}
