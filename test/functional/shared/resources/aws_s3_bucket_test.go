/*
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
*/

package resource_test

import (
	"testing"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_AWS_S3Bucket(t *testing.T) {
	template := "testdata/aws-s3-bucket.bicep"
	name := functional.GenerateS3BucketName()
	creationTimestamp := functional.GetCreationTimestamp()

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: name,
						Properties: map[string]any{
							"BucketName": name,
							"Tags": []any{
								map[string]any{
									"Key":   "testKey",
									"Value": "testValue",
								},
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

	test.Test(t)
}

func Test_AWS_S3Bucket_Existing(t *testing.T) {
	template := "testdata/aws-s3-bucket.bicep"
	templateExisting := "testdata/aws-s3-bucket-existing.bicep"
	name := functional.GenerateS3BucketName()
	creationTimestamp := functional.GetCreationTimestamp()

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: name,
						Properties: map[string]any{
							"BucketName": name,
							"Tags": []any{
								map[string]any{
									"Key":   "testKey",
									"Value": "testValue",
								},
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
			Executor:                               step.NewDeployExecutor(templateExisting, "bucketName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: name,
						Properties: map[string]any{
							"BucketName": name,
							"Tags": []any{
								map[string]any{
									"Key":   "testKey",
									"Value": "testValue",
								},
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

	test.Test(t)
}
