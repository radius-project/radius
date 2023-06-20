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
	"fmt"
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AWS_S3_Recipe(t *testing.T) {
	awsAccountId := os.Getenv("AWS_ACCOUNT_ID")
	awsRegion := os.Getenv("AWS_REGION")
	// Error the test if the required environment variables are not set
	// for running locally set the environment variables
	if awsAccountId == "" || awsRegion == "" {
		t.Error("This test needs the env variables AWS_ACCOUNT_ID and AWS_REGION to be set")
	}

	template := "testdata/corerp-resources-extenders-aws-s3-recipe.bicep"
	name := "corerp-resources-extenders-aws-s3-recipe"
	appName := "corerp-resources-extenders-aws-s3-recipe-app"
	bucketName := generateS3BucketName()

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				fmt.Sprintf("bucketName=%s", bucketName),
				functional.GetAWSAccountId(),
				functional.GetAWSRegion(),
				functional.GetRecipeRegistry(),
				functional.GetRecipeVersion(),
			),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-extenders-aws-s3-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-extenders-aws-s3-recipe-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-extenders-aws-s3-recipe",
						Type: validation.ExtendersResource,
						App:  appName,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderAWS,
								LocalID:  "RecipeResource0",
							},
						},
					},
				},
			},
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       bucketName,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: bucketName,
						Properties: map[string]any{
							"BucketName": bucketName,
						},
						SkipDeletion: true, // will be deleted by the recipe
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
