/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package resource_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AWS_MultiIdentifier_Resource(t *testing.T) {
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
