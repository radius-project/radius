// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	KinesisResourceType      = "AWS.Kinesis/Stream"
	MemoryDBResourceType     = "AWS.MemoryDB/Cluster"
	DBInstanceResourceType   = "AWS.RDS/DBInstance"
	MetricFilterResourceType = "AWS.Logs/MetricFilter"
	LogGroupResourceType     = "AWS.Logs/LogGroup"
)

type AWSResource struct {
	Type       string
	Name       string
	Identifier string
	Properties map[string]interface{}
}

type AWSResourceSet struct {
	Resources []AWSResource
}

func ValidateAWSResources(ctx context.Context, t *testing.T, expected *AWSResourceSet, client awsclient.AWSCloudControlClient) {
	for _, resource := range expected.Resources {
		resourceType := GetResourceTypeName(ctx, t, &resource)
		resourceResponse, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: to.StringPtr(resource.Identifier),
			TypeName:   &resourceType,
		})
		require.NoError(t, err)

		if resource.Properties != nil {
			var resourceResponseProperties map[string]interface{}
			err := json.Unmarshal([]byte(*resourceResponse.ResourceDescription.Properties), &resourceResponseProperties)
			require.NoError(t, err)

			assertFieldsArePresent(t, resource.Properties, resourceResponseProperties)
		}
	}
}

func DeleteAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client awsclient.AWSCloudControlClient) error {
	resourceType := GetResourceTypeName(ctx, t, resource)

	// Check if the resource exists
	_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.StringPtr(resource.Identifier),
		TypeName:   &resourceType,
	})
	notFound := awsclient.IsAWSResourceNotFound(err)
	if notFound {
		// Resource does not need to be deleted
		return nil
	}

	deleteOutput, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		Identifier: to.StringPtr(resource.Identifier),
		TypeName:   &resourceType,
	})
	if err != nil {
		return err
	}

	// Wait till the delete is complete
	maxWaitTime := 300 * time.Second
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(client)
	return waiter.Wait(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: deleteOutput.ProgressEvent.RequestToken,
	}, maxWaitTime)
}

func ValidateNoAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client awsclient.AWSCloudControlClient) {
	// Verify that the resource is indeed deleted
	resourceType := GetResourceTypeName(ctx, t, resource)
	_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.StringPtr(resource.Identifier),
		TypeName:   &resourceType,
	})

	notFound := awsclient.IsAWSResourceNotFound(err)
	require.True(t, notFound)
}

func GetResourceIdentifier(ctx context.Context, t *testing.T, resourceType string, name string) string {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := ""
	region := os.Getenv("AWS_REGION")

	credentialsProvider := credentials.NewStaticCredentialsProvider(accessKey, secretAccessKey, sessionToken)

	stsClient := sts.New(sts.Options{
		Region:      region,
		Credentials: credentialsProvider,
	})
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(t, err)

	return "/planes/aws/aws/accounts/" + *result.Account + "/regions/" + region + "/providers/" + resourceType + "/" + name
}

func GetResourceTypeName(ctx context.Context, t *testing.T, resource *AWSResource) string {
	id := GetResourceIdentifier(ctx, t, resource.Type, resource.Name)
	resourceID, err := resources.Parse(id)
	require.NoError(t, err)
	resourceType := resources.ToAWSResourceType(resourceID)
	return resourceType
}

// assertFieldsArePresent ensures that all fields in actual exist and are equivalent in expected
func assertFieldsArePresent(t *testing.T, actual interface{}, expected interface{}) {
	switch actual := actual.(type) {
	case map[string]interface{}:
		if expectedMap, ok := expected.(map[string]interface{}); ok {
			for k := range actual {
				assertFieldsArePresent(t, actual[k], expectedMap[k])
			}
		} else {
			require.Fail(t, "types of actual and expected do not match")
		}
	default:
		require.Equal(t, actual, expected)
	}
}
