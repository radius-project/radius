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
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"uuid"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
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

// Test_AWSRedeployWithAWSAssignedIdentifierUpdatesSameResource deploys a DHCP option set twice. AWS assigns its
// identifier, so the template cannot name it: the redeploy must find the set by its alias instead of creating another.
func Test_AWSRedeployWithAWSAssignedIdentifierUpdatesSameResource(t *testing.T) {
	template := "testdata/aws-mechanics-redeploy-dhcpoptions.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	ec2Client := newEC2Client(t)

	var dhcpOptionsID string
	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "dhcpOptionsName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				dhcpOptionsID = requireOneDHCPOptions(ctx, t, ec2Client, name)
			},
		},
		{
			Executor:                               step.NewDeployExecutor(template, "dhcpOptionsName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				require.Equal(t, dhcpOptionsID, requireOneDHCPOptions(ctx, t, ec2Client, name), "the redeploy replaced the DHCP option set")
			},
		},
	})

	// PreSetup runs after the AWS feature check, so a skipped test does not call AWS.
	test.PreSetup = func(_ context.Context, t *testing.T, _ rp.RPTest) {
		cleanupDHCPOptions(t, ec2Client, name)
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

				properties := map[string]any{}
				require.NoError(t, json.Unmarshal([]byte(*output.ResourceDescription.Properties), &properties))
				require.NotContains(t, properties, "RetentionInDays", "the redeploy did not clear the retention period")
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSRedeployWithUnchangedWriteOnlyPropertySucceeds deploys a key pair twice. Its KeyFormat is write-only, so AWS
// never returns it, and create-only, so AWS rejects any change to it. The unchanged redeploy must not send it again.
func Test_AWSRedeployWithUnchangedWriteOnlyPropertySucceeds(t *testing.T) {
	template := "testdata/aws-mechanics-redeploy-writeonly.bicep"
	name := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	keyPair := validation.AWSResource{
		Name:       name,
		Type:       validation.AWSEC2KeyPairResourceType,
		Identifier: name,
		Properties: map[string]any{"KeyName": name},
	}

	afterCreate := keyPair
	// Only the last step deletes the key pair, so cleanup does not delete it twice.
	afterCreate.SkipDeletion = true

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "keyPairName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{afterCreate}},
		},
		{
			Executor:                               step.NewDeployExecutor(template, "keyPairName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources:                           &validation.AWSResourceSet{Resources: []validation.AWSResource{keyPair}},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	test.Test(t)
}

// Test_AWSRecipeDeleteRemovesAllOutputResources deploys a recipe with two AWS output resources, then deletes the
// application. Both resources must be gone from AWS, including the DHCP option set whose identifier AWS assigned.
func Test_AWSRecipeDeleteRemovesAllOutputResources(t *testing.T) {
	template := "testdata/aws-mechanics-recipe-delete.bicep"
	name := "corerp-mechanics-aws-recipe-delete"
	appName := name + "-app"
	logGroupName := "radiusfunctionaltest-" + uuid.New().String()
	dhcpOptionsName := "radiusfunctionaltest-" + uuid.New().String()
	creationTimestamp := testutil.GetCreationTimestamp()
	ec2Client := newEC2Client(t)
	logGroup := validation.AWSResource{
		Name:       logGroupName,
		Type:       validation.AWSLogsLogGroupResourceType,
		Identifier: logGroupName,
	}

	// The output resource IDs end in the alias, not the AWS identifier.
	awsScope := fmt.Sprintf("/planes/aws/aws/accounts/%s/regions/%s/providers", os.Getenv("AWS_ACCOUNT_ID"), os.Getenv("AWS_REGION"))
	outputResources := []validation.OutputResourceResponse{
		{ID: awsScope + "/AWS.Logs/LogGroup/" + logGroupName},
		{ID: awsScope + "/AWS.EC2/DHCPOptions/" + dhcpOptionsName},
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
				"dhcpOptionsName="+dhcpOptionsName,
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
				requireOneDHCPOptions(ctx, t, ec2Client, dhcpOptionsName)
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

					ids, err := listDHCPOptions(ctx, ec2Client, dhcpOptionsName)
					require.NoError(c, err)
					assert.Empty(c, ids, "the DHCP option set %s is still in AWS", dhcpOptionsName)
				}, time.Minute, 10*time.Second)
			},
		},
	})

	rpTest.PreSetup = func(_ context.Context, t *testing.T, _ rp.RPTest) {
		cleanupDHCPOptions(t, ec2Client, dhcpOptionsName)
	}
	rpTest.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAWS}
	rpTest.Test(t)
}

func newEC2Client(t *testing.T) *ec2.Client {
	cfg, err := awsconfig.LoadDefaultConfig(t.Context())
	require.NoError(t, err)
	return ec2.NewFromConfig(cfg)
}

// listDHCPOptions returns the IDs of the DHCP option sets whose Name tag is name. The tests find DHCP option sets by
// tag because AWS assigns their identifiers.
func listDHCPOptions(ctx context.Context, client *ec2.Client, name string) ([]string, error) {
	ids := []string{}
	paginator := ec2.NewDescribeDhcpOptionsPaginator(client, &ec2.DescribeDhcpOptionsInput{
		Filters: []ec2types.Filter{{Name: new("tag:Name"), Values: []string{name}}},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, options := range page.DhcpOptions {
			ids = append(ids, *options.DhcpOptionsId)
		}
	}
	return ids, nil
}

// requireOneDHCPOptions requires that AWS has exactly one DHCP option set whose Name tag is name, and returns its ID.
// It waits for the set to appear, because EC2 reads are eventually consistent, but fails as soon as it sees two.
func requireOneDHCPOptions(ctx context.Context, t *testing.T, client *ec2.Client, name string) string {
	var ids []string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		ids, err = listDHCPOptions(ctx, client, name)
		require.NoError(c, err)
		require.NotEmpty(c, ids, "AWS has no DHCP option set named %s", name)
	}, time.Minute, 5*time.Second)
	require.Len(t, ids, 1, "AWS has more than one DHCP option set named %s: %v", name, ids)
	return ids[0]
}

// cleanupDHCPOptions deletes the DHCP option sets whose Name tag is name when the test ends, including when it fails.
func cleanupDHCPOptions(t *testing.T, client *ec2.Client, name string) {
	t.Cleanup(func() {
		// t.Context is canceled before cleanup runs.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute) //nolint:usetesting
		defer cancel()

		ids, err := listDHCPOptions(ctx, client, name)
		if err != nil {
			t.Errorf("failed to list the DHCP option sets named %s: %v", name, err)
			return
		}
		for _, id := range ids {
			t.Logf("deleting DHCP option set %s", id)
			_, err := client.DeleteDhcpOptions(ctx, &ec2.DeleteDhcpOptionsInput{DhcpOptionsId: &id})
			var apiErr smithy.APIError
			if err != nil && !(errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidDhcpOptionID.NotFound") {
				t.Errorf("failed to delete DHCP option set %s: %v", id, err)
			}
		}
	})
}
