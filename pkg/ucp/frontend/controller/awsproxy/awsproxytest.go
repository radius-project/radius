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
	testAWSResourceCollectionPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"
	testAWSSingleResourcePath     = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testAWSOperationResultsPath   = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/locations/us-west-2/operationResults/1234567"
	testAWSOperationStatusesPath  = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/locations/us-west-2/operationStatuses/2345678"
	testAWSResourceName           = "stream-1"
	testAWSResourceType           = "AWS.Kinesis/Stream"
	testAWSRequestToken           = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
)

type TestOptions struct {
	AWSClient     *awsclient.MockAWSClient
	StorageClient *store.MockStorageClient
}

// setupTest returns a TestOptions struct with mocked AWS and Storage clients
func setupTest(t *testing.T) TestOptions {
	mockCtrl := gomock.NewController(t)
	mockClient := awsclient.NewMockAWSClient(mockCtrl)
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	return TestOptions{
		AWSClient:     mockClient,
		StorageClient: mockStorageClient,
	}
}
