// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	awstest "github.com/project-radius/radius/test/functional/aws"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_KinesisStream(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	name := "my-stream"

	test := awstest.NewAWSTest(t, name, []awstest.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.KinesisResourceType,
					},
				},
			},
		},
	})

	test.Test(t)
}
