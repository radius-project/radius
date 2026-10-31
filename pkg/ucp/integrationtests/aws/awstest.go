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
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/ucp"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	ucp_aws_frontend "github.com/radius-project/radius/pkg/ucp/frontend/aws"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testProxyRequestAWSPath           = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testProxyRequestAWSCollectionPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"
	testAWSResourceName               = "stream-1"
	testProxyRequestAWSAsyncPath      = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global"
	testAWSRequestToken               = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID

	// testOwner is the owner the tests send on PUT and DELETE.
	testOwner = "deploy;/planes/radius/local/resourceGroups/rg;;"
)

func initializeAWSTest(t *testing.T) (*testhost.TestHost, *ucp_aws.MockAWSCloudControlClient, *ucp_aws.MockAWSCloudFormationClient) {
	ctrl := gomock.NewController(t)
	cloudControlClient := ucp_aws.NewMockAWSCloudControlClient(ctrl)
	cloudFormationClient := ucp_aws.NewMockAWSCloudFormationClient(ctrl)

	ucp := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.AWSClients.CloudControl = cloudControlClient
		options.AWSClients.CloudFormation = cloudFormationClient
		options.Config.AWS.FingerprintKeyFile = writeFingerprintKey(t)
		options.Modules = []modules.Initializer{ucp_aws_frontend.NewModule(options)}
	}))

	return ucp, cloudControlClient, cloudFormationClient
}

// initializeFakeAWSTest starts UCP with an in-memory Cloud Control, so the backend worker can finish operations.
func initializeFakeAWSTest(t *testing.T, schemas map[string]string) (*testhost.TestHost, *awsfake.CloudControl) {
	return initializeFakeAWSTestWithKey(t, schemas, writeFingerprintKey(t))
}

// initializeFakeAWSTestWithKey starts UCP with the fingerprint key in keyFile, which need not exist.
func initializeFakeAWSTestWithKey(t *testing.T, schemas map[string]string, keyFile string) (*testhost.TestHost, *awsfake.CloudControl) {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = 10*time.Millisecond, 10*time.Millisecond
	cc := awsfake.NewCloudControl()
	ucp := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.AWSClients.CloudControl = cc
		options.AWSClients.CloudFormation = &awsfake.CloudFormation{Schemas: schemas}
		options.Config.AWS.FingerprintKeyFile = keyFile
		options.Modules = []modules.Initializer{ucp_aws_frontend.NewModule(options)}
	}))
	return ucp, cc
}

// writeFingerprintKey writes a fingerprint key, like the one the Helm chart mounts, and returns its path.
func writeFingerprintKey(t *testing.T) string {
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{1}, ucp_aws.MinFingerprintKeyLength), 0o600))
	return path
}
