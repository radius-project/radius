// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/store"

	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
)

const (
	testAWSResourceCollectionPath   = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"
	testAWSSingleResourcePath       = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testAWSOperationResultsPath     = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/locations/us-west-2/operationResults/1234567"
	testAWSOperationStatusesPath    = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/locations/us-west-2/operationStatuses/2345678"
	testAWSResourceName             = "stream-1"
	testAWSResourceType             = "AWS.Kinesis/Stream"
	testAWSRequestToken             = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	testMultiIdentifierResourcePath = "/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.RedShift/EndpointAuthorization"
	testPrimaryIdentifier1          = "abc"
	testPrimaryIdentifier2          = "xyz"
	testMultiIdentifierResourceType = "AWS.RedShift/EndpointAuthorization"
	testHost                        = "localhost:5000"
	testlocationHeader              = "http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global/operationResults/79b9f0da-4882-4dc8-a367-6fd3bc122ded"
	testazureAsyncOpHeader          = "http://localhost:5000/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global/operationStatuses/79b9f0da-4882-4dc8-a367-6fd3bc122ded"
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
