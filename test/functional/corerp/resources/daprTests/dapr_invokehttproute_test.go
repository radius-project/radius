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

func Test_DaprInvokeHttpRoute(t *testing.T) {
	template := "../testdata/corerp-resources-dapr-httproute.bicep"
	name := "dapr-invokehttproute"
	appNamespace := "default-dapr-invokehttproute"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "dapr-frontend",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-backend",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-backend-httproute",
						Type: validation.DaprInvokeHttpRoutesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dapr-frontend"),
						validation.NewK8sPodForResource(name, "dapr-backend"),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}
