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
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	awsclient "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	KinesisResourceType = "AWS.Kinesis/Stream"
)

type AWSResource struct {
	Type       string
	Name       string
	Properties map[string]interface{}
}

type AWSResourceSet struct {
	Resources []AWSResource
}

func ValidateAWSResources(ctx context.Context, t *testing.T, expected *AWSResourceSet, client aws.AWSCloudControlClient) {
	for _, resource := range expected.Resources {
		resourceType := getResourceTypeName(t, &resource)
		resourceResponse, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: to.StringPtr(resource.Name),
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

func DeleteAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client aws.AWSCloudControlClient) error {
	resourceType := getResourceTypeName(t, resource)

	// Check if the resource exists
	_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.StringPtr(resource.Name),
		TypeName:   &resourceType,
	})
	notFound := aws.IsAWSResourceNotFound(err)
	if notFound {
		// Resource does not need to be deleted
		return nil
	}

	deleteOutput, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		Identifier: to.StringPtr(resource.Name),
		TypeName:   &resourceType,
	})
	require.NoError(t, err)

	// Wait till the delete is complete
	maxWaitTime := 300 * time.Second
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(client)
	err = waiter.Wait(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: deleteOutput.ProgressEvent.RequestToken,
	}, maxWaitTime)

	return err
}

func ValidateNoAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client aws.AWSCloudControlClient) {
	// Verify that the resource is indeed deleted
	resourceType := getResourceTypeName(t, resource)
	_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.StringPtr(resource.Name),
		TypeName:   &resourceType,
	})

	notFound := aws.IsAWSResourceNotFound(err)
	require.True(t, notFound)
}

func getResourceIdentifier(t *testing.T, resourceType string, name string) string {
	creds := credentials.NewStaticCredentials(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), "")
	awsConfig := awsclient.NewConfig().WithCredentials(creds).WithMaxRetries(3)
	mySession, err := session.NewSession(awsConfig)
	require.NoError(t, err)
	client := sts.New(mySession)
	result, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	region := os.Getenv("AWS_REGION")
	return "/planes/aws/aws/accounts/" + *result.Account + "/regions/" + region + "/providers/" + resourceType + "/" + name
}

func getResourceTypeName(t *testing.T, resource *AWSResource) string {
	id := getResourceIdentifier(t, resource.Type, resource.Name)
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
