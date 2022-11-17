// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/store"

	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
)

const (
	AWSKinesisStreamResourceType                    = "AWS.Kinesis/Stream"
	AWSKinesisStreamAWSResourceType                 = "AWS::Kinesis::Stream"
	AWSMemoryDBClusterResourceType                  = "AWS.MemoryDB/Cluster"
	AWSMemoryDBClusterAWSResourceType               = "AWS::MemoryDB::Cluster"
	AWSRedShiftEndpointAuthorizationResourceType    = "AWS.RedShift/EndpointAuthorization"
	AWSRedShiftEndpointAuthorizationAWSResourceType = "AWS::RedShift::EndpointAuthorization"

	testAWSRequestToken = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	testHost            = "localhost:5000"
)

type TestOptions struct {
	AWSCloudControlClient   *awsclient.MockAWSCloudControlClient
	AWSCloudFormationClient *awsclient.MockAWSCloudFormationClient
	StorageClient           *store.MockStorageClient
}

// setupTest returns a TestOptions struct with mocked AWS and Storage clients
func setupTest(t *testing.T) TestOptions {
	mockCtrl := gomock.NewController(t)
	mockCloudControlClient := awsclient.NewMockAWSCloudControlClient(mockCtrl)
	mockCloudFormationClient := awsclient.NewMockAWSCloudFormationClient(mockCtrl)
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	return TestOptions{
		AWSCloudControlClient:   mockCloudControlClient,
		AWSCloudFormationClient: mockCloudFormationClient,
		StorageClient:           mockStorageClient,
	}
}

type AWSTestResource struct {
	ResourceType          string
	AWSResourceType       string
	ResourceName          string
	CollectionPath        string
	SingleResourcePath    string
	OperationResultsPath  string
	OperationStatusesPath string
	LocationHeader        string
	AzureAsyncOpHeader    string
	TypeSchema            map[string]interface{}
	SerializedTypeSchema  string
	ARN                   string
}

func CreateAWSTestResource(resourceType string) *AWSTestResource {
	var resourceName string
	switch resourceType {
	case AWSKinesisStreamResourceType:
		resourceName = "test-stream"
	case AWSMemoryDBClusterResourceType:
		resourceName = "test-cluster"
	case AWSRedShiftEndpointAuthorizationResourceType:
		resourceName = "test-endpoint-authorization"
	default:
		return nil
	}

	// Add some random characters to the end of the resource name
	var suffixBuilder strings.Builder
	rand.Seed(time.Now().UnixNano())
	charset := "abcdefghijklmnopqrstuvwxyz"
	for i := 0; i < 5; i++ {
		suffixBuilder.WriteByte(charset[rand.Intn(len(charset))])
	}
	resourceNameSuffix := suffixBuilder.String()

	return CreateAWSTestResourceWithName(resourceType, fmt.Sprintf("%s-%s", resourceName, resourceNameSuffix))
}

func CreateAWSTestResourceWithName(resourceType, resourceName string) *AWSTestResource {
	var awsResourceType string
	var typeSchema map[string]interface{}
	var arn string
	var provider string
	switch resourceType {
	case AWSKinesisStreamResourceType:
		awsResourceType = AWSKinesisStreamAWSResourceType
		typeSchema = getMockKinesisStreamResourceTypeSchema()
		arn = fmt.Sprintf("arn:aws:kinesis:us-west-2:123456789012:stream:%s", resourceName)
		provider = "AWS.Kinesis"
	case AWSMemoryDBClusterResourceType:
		awsResourceType = AWSMemoryDBClusterAWSResourceType
		typeSchema = getMockMemoryDBClusterResourceTypeSchema()
		arn = fmt.Sprintf("arn:aws:memorydb:us-west-2:123456789012:cluster:%s", resourceName)
		provider = "AWS.MemoryDB"
	case AWSRedShiftEndpointAuthorizationResourceType:
		awsResourceType = AWSRedShiftEndpointAuthorizationAWSResourceType
		typeSchema = getMockRedShiftEndpointAuthorizationResourceTypeSchema()
		arn = fmt.Sprintf("arn:aws:redshift:us-west-2:123456789012:endpointauthorization:%s", resourceName)
		provider = "AWS.RedShift"
	default:
		return nil
	}

	serializedTypeSchema := serializeTypeSchema(typeSchema)

	return &AWSTestResource{
		ResourceType:          resourceType,
		AWSResourceType:       awsResourceType,
		ResourceName:          resourceName,
		CollectionPath:        fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s", resourceType),
		SingleResourcePath:    fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/%s", resourceType, resourceName),
		OperationResultsPath:  fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationResults/1234567", resourceType),
		OperationStatusesPath: fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationStatuses/2345678", resourceType),
		LocationHeader:        fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationResults/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		AzureAsyncOpHeader:    fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationStatuses/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		TypeSchema:            typeSchema,
		SerializedTypeSchema:  serializedTypeSchema,
		ARN:                   arn,
	}
}

func serializeTypeSchema(typeSchema map[string]interface{}) string {
	serialized, err := json.Marshal(typeSchema)
	if err != nil {
		return ""
	}
	return string(serialized)
}

func getMockKinesisStreamResourceTypeSchema() map[string]interface{} {
	return map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/Name",
		},
		"readOnlyProperties": []interface{}{
			"/properties/Arn",
		},
		"createOnlyProperties": []interface{}{
			"/properties/Name",
		},
	}
}

func getMockMemoryDBClusterResourceTypeSchema() map[string]interface{} {
	return map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterName",
		},
		"readOnlyProperties": []interface{}{
			"/properties/ClusterEndpoint/Address",
			"/properties/ClusterEndpoint/Port",
			"/properties/ARN",
		},
		"createOnlyProperties": []interface{}{
			"/properties/ClusterName",
			"/properties/Port",
		},
	}
}

func getMockRedShiftEndpointAuthorizationResourceTypeSchema() map[string]interface{} {
	return map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/ClusterIdentifier",
			"/properties/Account",
		},
	}
}
