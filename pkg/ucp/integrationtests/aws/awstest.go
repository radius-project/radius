// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
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
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err := api.Register(ctx, router,
		armrpc_controller.Options{
			BasePath:     basePath,
			DataProvider: provider,
		},
		&awsproxy.AWSOptions{
			AWSCloudControlClient:   cloudControlClient,
			AWSCloudFormationClient: cloudFormationClient,
		},
	)
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	return ucp, ucpClient, cloudControlClient, cloudFormationClient
}
