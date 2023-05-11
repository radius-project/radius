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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/stretchr/testify/require"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(httpClient *http.Client, baseURL string) Client {
	return Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

const (
	testProxyRequestAWSPath           = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1"
	testProxyRequestAWSCollectionPath = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream"
	testAWSResourceName               = "stream-1"
	testProxyRequestAWSAsyncPath      = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global"
	testAWSRequestToken               = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	basePath                          = "/apis/api.ucp.dev/v1alpha3"
)

func initializeTest(t *testing.T) (*httptest.Server, Client, *aws.MockAWSCloudControlClient, *aws.MockAWSCloudFormationClient) {
	ctrl := gomock.NewController(t)
	cloudControlClient := aws.NewMockAWSCloudControlClient(ctrl)
	cloudFormationClient := aws.NewMockAWSCloudFormationClient(ctrl)

	provider := dataprovider.NewMockDataStorageProvider(ctrl)
	provider.EXPECT().
		GetStorageClient(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	router := mux.NewRouter()
	router.Use(servicecontext.ARMRequestCtx(basePath, "global"))
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err := api.Register(ctx, router, controller.Options{
		BasePath: basePath,
		AWSOptions: controller.AWSOptions{
			AWSCloudControlClient:   cloudControlClient,
			AWSCloudFormationClient: cloudFormationClient,
		},
		Options: armrpc_controller.Options{
			DataProvider: provider,
		},
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	return ucp, ucpClient, cloudControlClient, cloudFormationClient
}
