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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the kinds of AWS types that Cloud Control doesn't fully manage: a type AWS can create and delete
// but not update, a read-only type AWS creates on its own, and a type with an extra read for values AWS's schema leaves
// out. Each deploys through rad deploy and the Deployment Engine.

// Test_AWSCreateAndDeleteOnlyTypeRejectsChange deploys a placement group, redeploys it unchanged, then changes its
// strategy. AWS can't update a placement group, so the unchanged redeploy must send nothing, and the change must fail
// with UCP's 400 and leave the group as it was.
func Test_AWSCreateAndDeleteOnlyTypeRejectsChange(t *testing.T) {
	template := "testdata/aws-mechanics-placementgroup.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	parameters := []string{"name=" + name, "creationTimestamp=" + creationTimestamp}

	var groupID string
	var since time.Time
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, append(parameters, "strategy=spread")...),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireAWSWrite(ctx, t, ct, name, msgCreating)
				groupID = requireOnePlacementGroup(ctx, t, newEC2Client(ctx, t), name, "spread")
				since = time.Now()
			},
		},
		{
			Executor:                               step.NewDeployExecutor(template, append(parameters, "strategy=spread")...),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireNoAWSWrites(ctx, t, ct, since, name)
				require.Equal(t, groupID, requireOnePlacementGroup(ctx, t, newEC2Client(ctx, t), name, "spread"), "the redeploy replaced the placement group")
				since = time.Now()
			},
		},
		{
			Executor: step.NewDeployErrorExecutor(template, requireErrorMessage(
				"can only be created and deleted, so these properties of the existing resource cannot change: Strategy",
			), append(parameters, "strategy=cluster")...),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireNoAWSWrites(ctx, t, ct, since, name)
				require.Equal(t, groupID, requireOnePlacementGroup(ctx, t, newEC2Client(ctx, t), name, "spread"), "the rejected change replaced the placement group")
			},
		},
	})

	// PreSetup runs after the AWS feature check, so a skipped test does not call AWS.
	test.PreSetup = func(ctx context.Context, t *testing.T, _ rp.RPTest) {
		cleanupPlacementGroups(t, newEC2Client(ctx, t), name)
	}
	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSReadOnlyTypeWithExisting references a public AMI with existing and checks the values UCP read reach another
// resource. AWS creates AMIs, so a template that declares one without existing must fail with UCP's 400.
func Test_AWSReadOnlyTypeWithExisting(t *testing.T) {
	name := "corerp-mechanics-aws-image-existing"
	var imageID, imageARN string

	deployExisting := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
		step.NewDeployExecutor("testdata/aws-mechanics-image-existing.bicep", "extenderName="+name, "imageArn="+imageARN).Execute(ctx, t, options)
	})
	deployExisting.Description = "deploy testdata/aws-mechanics-image-existing.bicep"

	deployCreate := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
		step.NewDeployErrorExecutor("testdata/aws-mechanics-image-create.bicep", requireErrorMessage(
			"The AWS resource type AWS::EC2::Image is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.",
		), "imageArn="+imageARN).Execute(ctx, t, options)
	})
	deployCreate.Description = "deploy testdata/aws-mechanics-image-create.bicep"

	extender := validation.RPResource{Name: name, Type: validation.ExtendersResource}
	var since time.Time
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: deployExisting,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{extender},
			},
			// The extender has no application to delete it with, so cleanupRadiusResources deletes it.
			SkipResourceDeletion: true,
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				extender, err := ct.Options.ManagementClient.GetResource(ctx, validation.ExtendersResource, name)
				require.NoError(t, err)
				assert.Equal(t, imageID, extender.Properties["imageId"])
				assert.Equal(t, "available", extender.Properties["imageState"])
				requireNoAWSWrites(ctx, t, ct, since, "image")
				since = time.Now()
			},
		},
		{
			Executor:                               deployCreate,
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireNoAWSWrites(ctx, t, ct, since, "image")
			},
		},
	})

	test.PreSetup = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		cleanupRadiusResources(t, ct.Options.ManagementClient, extender)
		since = time.Now()
		imageID = newestAmazonLinuxImage(ctx, t, newEC2Client(ctx, t))
		imageARN = fmt.Sprintf("arn:aws:ec2:%s::image/%s", os.Getenv("AWS_REGION"), imageID)
	}
	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSExtraReadPropertyRedeploySendsNothing deploys a recipe with an MSK Serverless cluster that outputs
// BootstrapBrokerStringSaslIam, which UCP reads with an extra call because AWS's schema leaves it out. The redeploy
// must send nothing to AWS: the broker addresses are in what UCP reads, not in the template.
func Test_AWSExtraReadPropertyRedeploySendsNothing(t *testing.T) {
	template := "testdata/aws-mechanics-msk-recipe.bicep"
	name := "corerp-mechanics-aws-msk"
	appName := name + "-app"
	clusterName := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	var subnetIDs []string

	deploy := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
		step.NewDeployExecutor(template,
			"clusterName="+clusterName,
			"subnetIds="+strings.Join(subnetIDs, ","),
			testutil.GetAWSAccountId(),
			testutil.GetAWSRegion(),
			testutil.GetBicepRecipeRegistry(),
			testutil.GetBicepRecipeVersion(),
		).Execute(ctx, t, options)
	})
	deploy.Description = "deploy " + template

	resourceSet := recipeResourceSet(name, appName, "AWS.MSK/ServerlessCluster", clusterName)
	var clusterARN, brokers string
	var since time.Time
	var rpTest rp.RPTest
	rpTest = rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:    beforeCleanup(deploy),
			RPResources: resourceSet,
			// cleanupRadiusResources deletes the application and environment within the test deadline.
			SkipResourceDeletion: true,
			SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireAWSWrite(ctx, t, ct, clusterName, msgCreating)
				client := newKafkaClient(ctx, t)
				clusterARN = requireOneMSKCluster(ctx, t, client, clusterName)
				brokers = mskBootstrapBrokers(ctx, t, client, clusterARN)
				requireExtenderValue(ctx, t, ct, name, "bootstrapBrokers", brokers)
				since = time.Now()
			},
		},
		{
			Executor:                               beforeCleanup(deploy),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				requireNoAWSWrites(ctx, t, ct, since, clusterName)
				require.Equal(t, clusterARN, requireOneMSKCluster(ctx, t, newKafkaClient(ctx, t), clusterName), "the redeploy replaced the cluster")
				requireExtenderValue(ctx, t, ct, name, "bootstrapBrokers", brokers)
			},
		},
		{
			Executor:                               beforeCleanup(deleteApplication(&rpTest, appName)),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				arns, err := listMSKClusters(ctx, newKafkaClient(ctx, t), clusterName)
				require.NoError(t, err)
				require.Empty(t, arns, "the cluster %s is still in AWS", clusterName)
			},
		},
	})

	rpTest.PreSetup = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		subnetIDs = createTestNetwork(ctx, t, newEC2Client(ctx, t), clusterName, creationTimestamp)
		// Cleanups run in reverse order: Radius deletes the app, which deletes the cluster, then the test makes sure
		// the cluster is gone, then deletes the network.
		cleanupMSKClusters(t, newKafkaClient(ctx, t), clusterName)
		cleanupRadiusResources(t, ct.Options.ManagementClient, validation.ResourcesInDeletionOrder(resourceSet)...)
	}
	rpTest.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	rpTest.Test(t)
}

// recipeResourceSet returns the environment, application, and extender a recipe template deploys, with the extender's
// one AWS output resource. Its ID ends in the alias, not the AWS identifier.
func recipeResourceSet(name string, appName string, awsType string, alias string) *validation.RPResourceSet {
	awsScope := fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s/providers", os.Getenv("AWS_ACCOUNT_ID"), os.Getenv("AWS_REGION"))
	return &validation.RPResourceSet{
		Resources: []validation.RPResource{
			{Name: name + "-env", Type: validation.EnvironmentsResource},
			{Name: appName, Type: validation.ApplicationsResource},
			{
				Name:            name,
				Type:            validation.ExtendersResource,
				App:             appName,
				OutputResources: []validation.OutputResourceResponse{{ID: awsScope + "/" + awsType + "/" + alias}},
			},
		},
	}
}

// deleteApplication returns a step that deletes the application, which deletes the AWS resources its recipes deployed.
// Cleanup can't do this: in CI it starts the delete and doesn't wait for it.
func deleteApplication(rpTest *rp.RPTest, appName string) step.FuncExecutor {
	executor := step.NewFuncExecutor(func(ctx context.Context, t *testing.T, _ test.TestOptions) {
		_, err := rpTest.Options.ManagementClient.DeleteApplication(ctx, appName, false)
		require.NoError(t, err)
	})
	executor.Description = "delete " + appName
	return executor
}

// requireExtenderValue requires that the extender has the recipe output value key, equal to want.
func requireExtenderValue(ctx context.Context, t *testing.T, ct rp.RPTest, name string, key string, want string) {
	require.NotEmpty(t, want, "AWS returned no value for %s", key)
	extender, err := ct.Options.ManagementClient.GetResource(ctx, validation.ExtendersResource, name)
	require.NoError(t, err)
	require.Equal(t, want, extender.Properties[key], "the extender's %s is not the value AWS returns", key)
}

// requireErrorMessage returns a deploy error check that requires message in the error or any of its details. The
// Deployment Engine nests the error UCP returned inside its own.
func requireErrorMessage(message string) func(*testing.T, *radcli.CLIError) {
	return func(t *testing.T, err *radcli.CLIError) {
		var messages []string
		var collect func(details *v1.ErrorDetails)
		collect = func(details *v1.ErrorDetails) {
			if details == nil {
				return
			}
			messages = append(messages, details.Message)
			for _, detail := range details.Details {
				collect(detail)
			}
		}
		collect(err.ErrorResponse.Error)
		for _, m := range messages {
			if strings.Contains(m, message) {
				return
			}
		}
		require.Failf(t, "the deploy failed with another error", "want an error containing %q, got:\n%s", message, strings.Join(messages, "\n"))
	}
}

func newEC2Client(ctx context.Context, t *testing.T) *ec2.Client {
	return ec2.NewFromConfig(loadAWSConfig(ctx, t))
}

func newKafkaClient(ctx context.Context, t *testing.T) *kafka.Client {
	return kafka.NewFromConfig(loadAWSConfig(ctx, t))
}
