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
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
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

func ValidateAWSResources(ctx context.Context, t *testing.T, expected *AWSResourceSet, client aws.AWSClient) {
	for _, resource := range expected.Resources {
		resourceType := getResourceTypeName(t, &resource)
		_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: to.StringPtr(resource.Name),
			TypeName:   &resourceType,
		})
		notFound := aws.IsAWSResourceNotFound(err)
		require.False(t, notFound)
	}
}

func DeleteAWSResource(ctx context.Context, t *testing.T, resource *AWSResource, client aws.AWSClient) {
	resourceType := getResourceTypeName(t, resource)
	deleteOutput, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		Identifier: to.StringPtr(resource.Name),
		TypeName:   &resourceType,
	})
	require.NoError(t, err)

	maxRetries := 60
	i := 0
	for i = 0; i < maxRetries; i++ {
		getRequestStatus, err := client.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
			RequestToken: deleteOutput.ProgressEvent.RequestToken,
		})
		require.NoError(t, err)
		if getRequestStatus.ProgressEvent.OperationStatus != types.OperationStatusInProgress {
			require.Equal(t, types.OperationStatusSuccess, getRequestStatus.ProgressEvent.OperationStatus, "Delete operation failed")
			break
		}
		time.Sleep(10 * time.Second)
	}
	require.Less(t, i, maxRetries, "Delete operation timed out")
	_, err = client.GetResource(ctx, &cloudcontrol.GetResourceInput{
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
	region := os.Getenv("AWS_DEFAULT_REGION")
	return "/planes/aws/aws/accounts/" + *result.Account + "/regions/" + region + "/providers/" + resourceType + "/" + name
}

func getResourceTypeName(t *testing.T, resource *AWSResource) string {
	id := getResourceIdentifier(t, resource.Type, resource.Name)
	resourceID, err := resources.Parse(id)
	require.NoError(t, err)
	resourceType := resources.ToARN(resourceID)
	return resourceType
}
