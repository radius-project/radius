/*
Copyright 2026 The Radius Authors.

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

package mechanics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests deploy through rad deploy and the Deployment Engine, so they cannot see which calls UCP makes to AWS.
// They check the result in AWS instead.

// Test_AWSRedeployWithAWSAssignedIdentifierUpdatesSameResource deploys a query definition twice. AWS assigns its
// identifier, so the template cannot name it: the redeploy must find it by its alias instead of creating another.
func Test_AWSRedeployWithAWSAssignedIdentifierUpdatesSameResource(t *testing.T) {
	template := "testdata/aws-mechanics-redeploy-querydefinition.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()

	var queryDefinitionID string
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "queryDefinitionName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				queryDefinitionID = requireOneQueryDefinition(ctx, t, ct.Options.AWSClient, name)
			},
		},
		{
			Executor:                               step.NewDeployExecutor(template, "queryDefinitionName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				require.Equal(t, queryDefinitionID, requireOneQueryDefinition(ctx, t, ct.Options.AWSClient, name), "the redeploy replaced the query definition")
			},
		},
	})

	// PreSetup runs after the AWS feature check, so a skipped test does not call AWS.
	test.PreSetup = func(_ context.Context, t *testing.T, ct rp.RPTest) {
		cleanupQueryDefinitions(t, ct.Options.AWSClient, name)
	}
	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSRedeployWithRemovedPropertyClearsProperty deploys a log group with a retention period, then redeploys it
// without one. Radius set the retention period, so removing it from the template must clear it in AWS.
func Test_AWSRedeployWithRemovedPropertyClearsProperty(t *testing.T) {
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	logGroup := validation.AWSResource{
		Name:       name,
		Type:       validation.AWSLogsLogGroupResourceType,
		Identifier: name,
	}

	withRetention := logGroup
	withRetention.Properties = map[string]any{"RetentionInDays": float64(7)}
	// Only the last step deletes the log group, so cleanup does not delete it twice.
	withRetention.SkipDeletion = true

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor("testdata/aws-mechanics-redeploy-withupdatedresource.bicep", "logGroupName="+name, "creationTimestamp="+creationTimestamp, "retentionInDays=7"),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{withRetention}},
		},
		{
			Executor:                               step.NewDeployExecutor("testdata/aws-mechanics-redeploy-removedproperty.bicep", "logGroupName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{logGroup}},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				typeName, err := validation.GetResourceTypeName(ctx, &logGroup)
				require.NoError(t, err)
				output, err := ct.Options.AWSClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
					TypeName:   &typeName,
					Identifier: &logGroup.Identifier,
				})
				require.NoError(t, err)
				require.NotNil(t, output.ResourceDescription)
				require.NotNil(t, output.ResourceDescription.Properties)

				properties := map[string]any{}
				require.NoError(t, json.Unmarshal([]byte(*output.ResourceDescription.Properties), &properties))
				// A read without the name is incomplete, so it cannot show that the retention period is gone.
				require.Equal(t, name, properties["LogGroupName"], "AWS returned incomplete properties for the log group")
				require.NotContains(t, properties, "RetentionInDays", "the redeploy did not clear the retention period")
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSRedeployWithUnchangedWriteOnlyPropertySucceeds deploys a bucket twice. Its BucketNamespace is write-only, so
// AWS never returns it, and create-only, so AWS rejects any change to it. The unchanged redeploy must not send it again.
func Test_AWSRedeployWithUnchangedWriteOnlyPropertySucceeds(t *testing.T) {
	template := "testdata/aws-mechanics-redeploy-writeonly.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	bucket := validation.AWSResource{
		Name:       name,
		Type:       validation.AWSS3BucketResourceType,
		Identifier: name,
		Properties: map[string]any{"BucketName": name},
	}

	afterCreate := bucket
	// Only the last step deletes the bucket, so cleanup does not delete it twice.
	afterCreate.SkipDeletion = true

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{afterCreate}},
		},
		{
			Executor:                               step.NewDeployExecutor(template, "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{bucket}},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSRecipeDeleteRemovesAllOutputResources deploys a recipe with two AWS output resources, then deletes the
// application. Both resources must be gone from AWS, including the query definition whose identifier AWS assigned.
func Test_AWSRecipeDeleteRemovesAllOutputResources(t *testing.T) {
	template := "testdata/aws-mechanics-recipe-delete.bicep"
	name := "corerp-mechanics-aws-recipe-delete"
	appName := name + "-app"
	logGroupName := "radiusfunctionaltest-" + uuid.New().String()
	queryDefinitionName := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	logGroup := validation.AWSResource{
		Name:       logGroupName,
		Type:       validation.AWSLogsLogGroupResourceType,
		Identifier: logGroupName,
	}

	// The output resource IDs end in the alias, not the AWS identifier.
	awsScope := fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s/providers", os.Getenv("AWS_ACCOUNT_ID"), os.Getenv("AWS_REGION"))
	outputResources := []validation.OutputResourceResponse{
		{ID: awsScope + "/AWS.Logs/LogGroup/" + logGroupName},
		{ID: awsScope + "/AWS.Logs/QueryDefinition/" + queryDefinitionName},
	}

	var rpTest rp.RPTest
	deleteApp := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, _ test.TestOptions) {
		_, err := rpTest.Options.ManagementClient.DeleteApplication(ctx, appName, false)
		require.NoError(t, err)
	})
	deleteApp.Description = "delete " + appName

	rpTest = rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				"logGroupName="+logGroupName,
				"queryDefinitionName="+queryDefinitionName,
				"creationTimestamp="+creationTimestamp,
				testutil.GetAWSAccountId(),
				testutil.GetAWSRegion(),
				testutil.GetBicepRecipeRegistry(),
				testutil.GetBicepRecipeVersion(),
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name + "-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name:            name,
						Type:            validation.ExtendersResource,
						App:             appName,
						OutputResources: outputResources,
					},
				},
			},
			// Cleanup deletes the log group if the test fails before the recipe deletes it.
			AWSResources:         &validation.AWSResourceSet{Resources: []validation.AWSResource{logGroup}},
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireOneQueryDefinition(ctx, t, ct.Options.AWSClient, queryDefinitionName)
			},
		},
		{
			Executor:                               deleteApp,
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				require.EventuallyWithT(t, func(c *assert.CollectT) {
					notFound, err := validation.IsAWSResourceNotFound(ctx, &logGroup, ct.Options.AWSClient)
					require.NoError(c, err)
					assert.True(c, notFound, "the log group %s is still in AWS", logGroupName)

					ids, err := listQueryDefinitions(ctx, ct.Options.AWSClient, queryDefinitionName)
					require.NoError(c, err)
					assert.Empty(c, ids, "the query definition %s is still in AWS", queryDefinitionName)
				}, time.Minute, 10*time.Second)
			},
		},
	})

	rpTest.PreSetup = func(_ context.Context, t *testing.T, ct rp.RPTest) {
		cleanupQueryDefinitions(t, ct.Options.AWSClient, queryDefinitionName)
	}
	rpTest.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	rpTest.Test(t)
}

// queryDefinitionTypeName is the Cloud Control type name of AWS.Logs/QueryDefinition.
const queryDefinitionTypeName = "AWS::Logs::QueryDefinition"

// listQueryDefinitions returns the IDs of the query definitions named name. The tests find query definitions by name
// because AWS assigns their identifiers.
func listQueryDefinitions(ctx context.Context, client awsclient.AWSCloudControlClient, name string) ([]string, error) {
	return validation.ListAWSResourcesByProperty(ctx, client, queryDefinitionTypeName, "Name", name)
}

// requireOneQueryDefinition requires that AWS has exactly one query definition named name, and returns its ID. It waits
// for the query definition to appear, because AWS reads are eventually consistent, but fails as soon as it sees two.
func requireOneQueryDefinition(ctx context.Context, t *testing.T, client awsclient.AWSCloudControlClient, name string) string {
	var ids []string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		ids, err = listQueryDefinitions(ctx, client, name)
		require.NoError(c, err)
		require.NotEmpty(c, ids, "AWS has no query definition named %s", name)
	}, time.Minute, 5*time.Second)
	require.Len(t, ids, 1, "AWS has more than one query definition named %s: %v", name, ids)
	return ids[0]
}

// cleanupQueryDefinitions deletes the query definitions named name when the test ends, including when it fails.
func cleanupQueryDefinitions(t *testing.T, client awsclient.AWSCloudControlClient, name string) {
	t.Cleanup(func() {
		// t.Context is canceled before cleanup runs.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) //nolint:usetesting
		defer cancel()

		ids, err := listQueryDefinitions(ctx, client, name)
		if err != nil {
			t.Errorf("failed to list the query definitions named %s: %v", name, err)
			return
		}
		for _, id := range ids {
			t.Logf("deleting query definition %s", id)
			resource := validation.AWSResource{Name: id, Type: validation.AWSLogsQueryDefinitionResourceType, Identifier: id}
			if err := validation.DeleteAWSResource(ctx, &resource, client); err != nil {
				t.Errorf("failed to delete query definition %s: %v", id, err)
			}
		}
	})
}
