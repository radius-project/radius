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
	"testing"

	"os"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_Extender_RecipeAWS(t *testing.T) {
	awsAccountID := os.Getenv("AWS_ACCOUNT_ID")
	awsRegion := os.Getenv("AWS_REGION")
	// Error the test if the required environment variables are not set
	// for running locally set the environment variables
	if awsAccountID == "" || awsRegion == "" {
		t.Error("This test needs the env variables AWS_ACCOUNT_ID and AWS_REGION to be set")
	}

	template := "testdata/corerp-resources-extender-aws-s3-recipe.bicep"
	name := "corerp-resources-extenders-aws-s3-recipe"
	appName := "corerp-resources-extenders-aws-s3-recipe-app"
	bucketName := testutil.GenerateS3BucketName()
	bucketID := fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s/providers/AWS.S3/Bucket/%s", awsAccountID, awsRegion, bucketName)
	creationTimestamp := testutil.GetCreationTimestamp()

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				"bucketName="+bucketName,
				"creationTimestamp="+creationTimestamp,
				testutil.GetAWSAccountId(),
				testutil.GetAWSRegion(),
				testutil.GetBicepRecipeRegistry(),
				testutil.GetBicepRecipeVersion(),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
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
								ID: bucketID,
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
							"Tags": []any{
								map[string]any{
									"Key":   "RadiusCreationTimestamp",
									"Value": creationTimestamp,
								},
							},
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
