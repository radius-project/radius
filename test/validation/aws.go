// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
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
	Properties string
}

type AWSResourceSet struct {
	Resources []AWSResource
}

func ValidateAWSResources(ctx context.Context, t *testing.T, expected *AWSResourceSet, client aws.AWSCloudControlClient) {
	for _, resource := range expected.Resources {
		resourceType := getResourceTypeName(t, &resource)
		_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: to.StringPtr(resource.Name),
			TypeName:   &resourceType,
		})
		require.NoError(t, err)
	}
}

func DeleteAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client aws.AWSCloudControlClient) error {
	resourceType := getResourceTypeName(t, resource)
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
