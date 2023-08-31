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

package aws

// Tests that test with Mock RP functionality and UCP Server

import (
	"testing"

	"github.com/golang/mock/gomock"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	ucp_aws_frontend "github.com/radius-project/radius/pkg/ucp/frontend/aws"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/radius-project/radius/pkg/ucp/secret"
	"github.com/radius-project/radius/pkg/ucp/store"
)

const (
	testProxyRequestAWSPath           = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testProxyRequestAWSCollectionPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"
	testAWSResourceName               = "stream-1"
	testProxyRequestAWSAsyncPath      = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global"
	testAWSRequestToken               = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
)

func initializeAWSTest(t *testing.T) (*testserver.TestServer, *store.MockStorageClient, *secret.MockClient, *ucp_aws.MockAWSCloudControlClient, *ucp_aws.MockAWSCloudFormationClient) {
	ctrl := gomock.NewController(t)
	cloudControlClient := ucp_aws.NewMockAWSCloudControlClient(ctrl)
	cloudFormationClient := ucp_aws.NewMockAWSCloudFormationClient(ctrl)

	ucp, storeClient, secretClient := testserver.StartWithMocks(t, func(options modules.Options) []modules.Initializer {
		module := ucp_aws_frontend.NewModule(options)
		module.AWSClients.CloudControl = cloudControlClient
		module.AWSClients.CloudFormation = cloudFormationClient
		return []modules.Initializer{module}
	})

	return ucp, storeClient, secretClient, cloudControlClient, cloudFormationClient
}
