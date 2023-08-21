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
package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/to"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	AWSS3BucketResourceType         = "AWS.S3/Bucket"
	AWSMemoryDBClusterResourceType  = "AWS.MemoryDB/Cluster"
	AWSRDSDBInstanceResourceType    = "AWS.RDS/DBInstance"
	AWSLogsMetricFilterResourceType = "AWS.Logs/MetricFilter"
	AWSLogsLogGroupResourceType     = "AWS.Logs/LogGroup"
)

type AWSResource struct {
	// Type of the resource (e.g. AWS.S3/Bucket)
	Type string
	// Name of the resource (e.g. my-bucket)
	Name string
	// Primary identifier of the resource (e.g. my-bucket)
	Identifier string
	// Properties of the resource
	Properties map[string]any
	// Determines whether or not the resource should be deleted after the test
	SkipDeletion bool
}

type AWSResourceSet struct {
	Resources []AWSResource
}

// ValidateAWSResources checks that the expected AWS resources exist and have the expected properties.
func ValidateAWSResources(ctx context.Context, t *testing.T, expected *AWSResourceSet, client awsclient.AWSCloudControlClient) {
	for _, resource := range expected.Resources {
		resourceType, err := GetResourceTypeName(ctx, &resource)
		require.NoError(t, err)

		resourceResponse, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
			Identifier: to.Ptr(resource.Identifier),
			TypeName:   &resourceType,
		})
		require.NoError(t, err)

		if resource.Properties != nil {
			var resourceResponseProperties map[string]any
			err := json.Unmarshal([]byte(*resourceResponse.ResourceDescription.Properties), &resourceResponseProperties)
			require.NoError(t, err)

			assertFieldsArePresent(t, resource.Properties, resourceResponseProperties)
		}
	}
}

// DeleteAWSResource checks if the given AWS resource exists, deletes it if it does and waits until the delete is complete,
//
//	returning an error if any of these steps fail.
func DeleteAWSResource(ctx context.Context, resource *AWSResource, client awsclient.AWSCloudControlClient) error {
	resourceType, err := GetResourceTypeName(ctx, resource)
	if err != nil {
		return err
	}

	// Check if the resource exists
	_, err = client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.Ptr(resource.Identifier),
		TypeName:   &resourceType,
	})

	notFound := awsclient.IsAWSResourceNotFoundError(err)
	if notFound {
		// Resource does not need to be deleted
		return nil
	}

	deleteOutput, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		Identifier: to.Ptr(resource.Identifier),
		TypeName:   &resourceType,
	})
	if err != nil {
		return err
	}

	// Wait till the delete is complete
	maxWaitTime := 300 * time.Second
	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(client, func(options *cloudcontrol.ResourceRequestSuccessWaiterOptions) {
		options.LogWaitAttempts = true
	})
	err = waiter.Wait(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: deleteOutput.ProgressEvent.RequestToken,
	}, maxWaitTime)
	if err != nil {
		return fmt.Errorf("failed to delete resource %s after %s: %w", resource.Identifier, maxWaitTime, err)
	}

	return nil
}

// IsAWSResourceNotFound checks if the given AWS resource is not found.
func IsAWSResourceNotFound(ctx context.Context, resource *AWSResource, client awsclient.AWSCloudControlClient) (bool, error) {
	// Verify that the resource is indeed deleted
	resourceType, err := GetResourceTypeName(ctx, resource)
	if err != nil {
		return false, err
	}

	_, err = client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		Identifier: to.Ptr(resource.Identifier),
		TypeName:   &resourceType,
	})

	return awsclient.IsAWSResourceNotFoundError(err), err

}

// GetResourceIdentifier retrieves the identifier of a resource from the environment variables and the context.
func GetResourceIdentifier(ctx context.Context, resourceType string, name string) (string, error) {
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
	if err != nil {
		return "", err
	}

	return "/planes/aws/aws/accounts/" + *result.Account + "/regions/" + region + "/providers/" + resourceType + "/" + name, nil
}

// GetResourceTypeName retrieves the AWS resource type name from the resource identifier and context. It returns an
// error if the resource identifier or context is invalid.
func GetResourceTypeName(ctx context.Context, resource *AWSResource) (string, error) {
	id, err := GetResourceIdentifier(ctx, resource.Type, resource.Name)
	if err != nil {
		return "", err
	}

	resourceID, err := resources.Parse(id)
	if err != nil {
		return "", err
	}

	resourceType := resources.ToAWSResourceType(resourceID)
	return resourceType, nil
}

// assertFieldsArePresent ensures that all fields in actual exist and are equivalent in expected
func assertFieldsArePresent(t *testing.T, actual any, expected any) {
	switch actual := actual.(type) {
	case map[string]any:
		if expectedMap, ok := expected.(map[string]any); ok {
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
