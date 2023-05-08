/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package awsproxy

import (
	"encoding/json"
	"fmt"
	"testing"

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
	testScheme          = "http"
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
	ARN                   string
	CollectionPath        string
	SingleResourcePath    string
	OperationResultsPath  string
	OperationStatusesPath string
	LocationHeader        string
	AzureAsyncOpHeader    string
	Schema                string
}

func CreateKinesisStreamTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSKinesisStreamResourceType
	awsResourceType := AWSKinesisStreamAWSResourceType
	provider := "AWS.Kinesis"
	arn := fmt.Sprintf("arn:aws:kinesis:us-west-2:123456789012:stream:%s", resourceName)
	typeSchema := getMockKinesisStreamResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

func CreateMemoryDBClusterTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSMemoryDBClusterResourceType
	awsResourceType := AWSMemoryDBClusterAWSResourceType
	provider := "AWS.MemoryDB"
	arn := fmt.Sprintf("arn:aws:memorydb:us-west-2:123456789012:cluster:%s", resourceName)
	typeSchema := getMockMemoryDBClusterResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

func CreateRedshiftEndpointAuthorizationTestResource(resourceName string) *AWSTestResource {
	resourceType := AWSRedShiftEndpointAuthorizationResourceType
	awsResourceType := AWSRedShiftEndpointAuthorizationAWSResourceType
	provider := "AWS.Redshift"
	arn := fmt.Sprintf("arn:aws:redshift:us-west-2:123456789012:endpointauthorization:%s", resourceName)
	typeSchema := getMockRedShiftEndpointAuthorizationResourceTypeSchema()

	return CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn, typeSchema)
}

func CreateAWSTestResource(resourceType, awsResourceType, resourceName, provider, arn string, typeSchema map[string]any) *AWSTestResource {
	serialized, err := json.Marshal(typeSchema)
	if err != nil {
		return nil
	}
	schema := string(serialized)

	return &AWSTestResource{
		ResourceType:          resourceType,
		AWSResourceType:       awsResourceType,
		ResourceName:          resourceName,
		ARN:                   arn,
		CollectionPath:        fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s", resourceType),
		SingleResourcePath:    fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/%s", resourceType, resourceName),
		OperationResultsPath:  fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationResults/1234567", resourceType),
		OperationStatusesPath: fmt.Sprintf("/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/us-west-2/operationStatuses/2345678", resourceType),
		LocationHeader:        fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationResults/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		AzureAsyncOpHeader:    fmt.Sprintf("http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/%s/locations/global/operationStatuses/79b9f0da-4882-4dc8-a367-6fd3bc122ded", provider),
		Schema:                schema,
	}
}

func getMockKinesisStreamResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/Name",
		},
		"readOnlyProperties": []any{
			"/properties/Arn",
		},
		"createOnlyProperties": []any{
			"/properties/Name",
		},
	}
}

func getMockMemoryDBClusterResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/ClusterName",
		},
		"readOnlyProperties": []any{
			"/properties/ClusterEndpoint/Address",
			"/properties/ClusterEndpoint/Port",
			"/properties/ARN",
		},
		"createOnlyProperties": []any{
			"/properties/ClusterName",
			"/properties/Port",
		},
	}
}

func getMockRedShiftEndpointAuthorizationResourceTypeSchema() map[string]any {
	return map[string]any{
		"primaryIdentifier": []any{
			"/properties/ClusterIdentifier",
			"/properties/Account",
		},
	}
}
