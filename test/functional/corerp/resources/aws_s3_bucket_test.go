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
	template := "testdata/s3-bucket.bicep"
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
							"AccessControl": "Private",
						},
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_AWS_S3Bucket_Existing(t *testing.T) {
	template := "testdata/s3-bucket.bicep"
	templateExisting := "testdata/s3-bucket-existing.bicep"
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
							"AccessControl": "Private",
						},
					},
				},
			},
		},
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
							"AccessControl": "Private",
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
