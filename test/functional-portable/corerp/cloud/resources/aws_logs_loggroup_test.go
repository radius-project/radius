/*
Copyright 2025 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_AWS_LogsLogGroup(t *testing.T) {
	template := "testdata/aws-logs-loggroup.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "logGroupName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSLogsLogGroupResourceType,
						Identifier: name,
						Properties: map[string]any{
							"LogGroupName":    name,
							"RetentionInDays": 7,
							"Tags": []any{
								map[string]any{
									"Key":   "RadiusCreationTimestamp",
									"Value": creationTimestamp,
								},
							},
						},
					},
				},
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

func Test_AWS_LogsLogGroup_Existing(t *testing.T) {
	template := "testdata/aws-logs-loggroup.bicep"
	templateExisting := "testdata/aws-logs-loggroup-existing.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "logGroupName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSLogsLogGroupResourceType,
						Identifier: name,
						Properties: map[string]any{
							"LogGroupName":    name,
							"RetentionInDays": 7,
							"Tags": []any{
								map[string]any{
									"Key":   "RadiusCreationTimestamp",
									"Value": creationTimestamp,
								},
							},
						},
					},
				},
			},
		},
		// The following step deploys an existing resource and validates that it retrieves the same
		// resource as was deployed above
		{
			Executor:                               step.NewDeployExecutor(templateExisting, "logGroupName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSLogsLogGroupResourceType,
						Identifier: name,
						Properties: map[string]any{
							"LogGroupName":    name,
							"RetentionInDays": 7,
							"Tags": []any{
								map[string]any{
									"Key":   "RadiusCreationTimestamp",
									"Value": creationTimestamp,
								},
							},
						},
					},
				},
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}
