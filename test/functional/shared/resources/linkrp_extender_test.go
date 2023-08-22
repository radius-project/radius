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

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_LinkRP_Extender_Manual(t *testing.T) {
	template := "testdata/linkrp-resources-extender.bicep"
	name := "linkrp-resources-extender"
	appNamespace := "default-linkrp-resources-extender"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "extr-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "extr-twilio-old",
						Type: validation.O_ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "extr-ctnr-old"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_LinkRP_Extender_Recipe(t *testing.T) {
	template := "testdata/linkrp-resources-extender-recipe.bicep"
	name := "linkrp-resources-extender-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "linkrp-resources-extender-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "extender-recipe-old",
						Type: validation.O_ExtendersResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}

func Test_LinkRP_Extender_RecipeAWS(t *testing.T) {
	awsAccountId := os.Getenv("AWS_ACCOUNT_ID")
	awsRegion := os.Getenv("AWS_REGION")
	// Error the test if the required environment variables are not set
	// for running locally set the environment variables
	if awsAccountId == "" || awsRegion == "" {
		t.Error("This test needs the env variables AWS_ACCOUNT_ID and AWS_REGION to be set")
	}

	template := "testdata/linkrp-resources-extender-aws-s3-recipe.bicep"
	name := "linkrp-resources-extenders-aws-s3-recipe"
	appName := "linkrp-resources-extenders-aws-s3-recipe-app"
	bucketName := generateS3BucketName()

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				fmt.Sprintf("bucketName=%s", bucketName),
				functional.GetAWSAccountId(),
				functional.GetAWSRegion(),
				functional.GetBicepRecipeRegistry(),
				functional.GetBicepRecipeVersion(),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "linkrp-resources-extenders-aws-s3-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "linkrp-resources-extenders-aws-s3-recipe-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "linkrp-resources-extenders-aws-s3-recipe",
						Type: validation.O_ExtendersResource,
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
