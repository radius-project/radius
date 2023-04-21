// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprComponentNameConflict(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-component-name-conflict.bicep"
	name := "corerp-resources-dapr-component-name-conflict"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, v1.CodeInternal),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-component-name-conflict",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}
