// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AWS_MultiIdentifier_Resource(t *testing.T) {
	t.Skip()
	template := "testdata/aws-multi-identifier.bicep"
	filterName := "ms" + uuid.New().String()
	logGroupName := "ms" + uuid.New().String()
	testName := "ms" + uuid.New().String()

	test := corerp.NewCoreRPTest(t, testName, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "filterName="+filterName, "logGroupName="+logGroupName),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       logGroupName,
						Type:       validation.AWSLogsLogGroupResourceType,
						Identifier: logGroupName,
					},
					{
						Name:       filterName,
						Type:       validation.AWSLogsMetricFilterResourceType,
						Identifier: logGroupName + "|" + filterName,
					},
				},
			},
		},
	})

	test.Test(t)
}
