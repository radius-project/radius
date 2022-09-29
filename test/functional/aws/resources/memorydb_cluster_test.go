// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	awstest "github.com/project-radius/radius/test/functional/aws"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_MemoryDB_Cluster(t *testing.T) {
	template := "testdata/aws-memorydb.bicep"
	name := "my-test-cluster"

	test := awstest.NewAWSTest(t, name, []awstest.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.MemoryDBResourceType,
					},
				},
			},
		},
	})

	test.Test(t)
}
