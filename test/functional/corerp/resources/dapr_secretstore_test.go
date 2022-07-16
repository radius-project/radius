// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// FIXME: If I set `DAPR_GRPC_PORT`, I get the following error:
// failed to create Dapr client -  error creating connection to '127.0.0.1:3000': context deadline exceeded: context deadline exceeded
// If I don't set it, it says nil port.
// Bicep needs to be updated.
func Test_DaprSecretStoreGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-secretstore-generic.bicep"
	name := "corerp-resources-dapr-secretstore-generic"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-secretstore-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "myapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "secretstore-generic",
						Type: validation.DaprSecretStoreResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "myapp"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
