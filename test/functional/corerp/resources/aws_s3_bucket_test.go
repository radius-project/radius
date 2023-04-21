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

func Test_AWS_S3Bucket(t *testing.T) {
	t.Skip()
	template := "testdata/aws-s3-bucket.bicep"
	name := generateS3BucketName()

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name),
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
	name := generateS3BucketName()

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name),
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
							},
						},
					},
				},
			},
		},
	})

	test.Test(t)
}

func generateS3BucketName() string {
	return "radiusfunctionaltestbucket-" + uuid.New().String()
}
