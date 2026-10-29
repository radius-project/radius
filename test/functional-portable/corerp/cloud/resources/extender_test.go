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
	"context"
	"fmt"
	"strings"
	"testing"

	"os"

	"uuid"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

func Test_Extender_RecipeAWS_LogGroup(t *testing.T) {
	awsAccountID := os.Getenv("AWS_ACCOUNT_ID")
	awsRegion := os.Getenv("AWS_REGION")
	// Error the test if the required environment variables are not set
	// for running locally set the environment variables
	if awsAccountID == "" || awsRegion == "" {
		t.Error("This test needs the env variables AWS_ACCOUNT_ID and AWS_REGION to be set")
	}

	template := "testdata/corerp-resources-extender-aws-logs-recipe.bicep"
	name := "corerp-resources-extenders-aws-logs-recipe"
	appName := "corerp-resources-extenders-aws-logs-recipe-app"
	logGroupName := "radiusfunctionaltest-" + uuid.New().String()
	logGroupID := fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s/providers/AWS.Logs/LogGroup/%s", awsAccountID, awsRegion, logGroupName)
	// Cloud Control returns the log group ARN with a trailing ":*", which Radius stores without the suffix.
	logGroupARN := fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", awsRegion, awsAccountID, logGroupName)
	creationTimestamp := testutil.GetCreationTimestamp()

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				"logGroupName="+logGroupName,
				"creationTimestamp="+creationTimestamp,
				testutil.GetAWSAccountId(),
				testutil.GetAWSRegion(),
				testutil.GetBicepRecipeRegistry(),
				testutil.GetBicepRecipeVersion(),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-extenders-aws-logs-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-extenders-aws-logs-recipe-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-extenders-aws-logs-recipe",
						Type: validation.ExtendersResource,
						App:  appName,
						OutputResources: []validation.OutputResourceResponse{
							{
								ID:                 logGroupID,
								ProviderResourceID: logGroupARN,
							},
						},
					},
				},
			},
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       logGroupName,
						Type:       validation.AWSLogsLogGroupResourceType,
						Identifier: logGroupName,
						Properties: map[string]any{
							"LogGroupName": logGroupName,
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
			// Verify that the app graph links the log group to the CloudWatch log groups page in the AWS console.
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				client, ok := ct.Options.ManagementClient.(*clients.UCPApplicationsManagementClient)
				require.True(t, ok, "expected UCPApplicationsManagementClient")

				appGraphClient, err := v20231001preview.NewApplicationsClient(&aztoken.AnonymousCredential{}, client.ClientOptions)
				require.NoError(t, err)

				res, err := appGraphClient.GetGraph(ctx, client.RootScope, appName, v20231001preview.GetGraphRequest{}, nil)
				require.NoError(t, err)

				var portalURL *string
				found := false
				for _, r := range res.Resources {
					if r == nil || r.Type == nil || !strings.EqualFold(*r.Type, "Applications.Core/extenders") {
						continue
					}
					for _, or := range r.OutputResources {
						if or != nil && or.ID != nil && strings.EqualFold(*or.ID, logGroupID) {
							portalURL = or.PortalURL
							found = true
						}
					}
				}

				require.True(t, found, "expected the extender in the graph to have the log group output resource %s", logGroupID)
				require.NotNil(t, portalURL, "expected the log group output resource to carry a portalUrl")
				expectedPortalURL := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudwatch/home?region=%s#logsV2:log-groups", awsRegion, awsRegion)
				require.Equal(t, expectedPortalURL, *portalURL)
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}
