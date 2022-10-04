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

func Test_KinesisStream(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	name := "my-stream"
	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                     step.NewDeployExecutor(template),
			SkipRadiusResourceValidation: true,
			SkipObjectValidation:         true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.KinesisResourceType,
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
