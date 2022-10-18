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
	"github.com/project-radius/radius/pkg/ucp/aws"
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
	testAWSResourceType               = "AWS.Kinesis/Stream"
	testProxyRequestAWSAsyncPath      = "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/locations/global"
	testAWSPlaneID                    = "/planes/aws/aws"
	testAWSRequestToken               = "79B9F0DA-4882-4DC8-A367-6FD3BC122DED" // Random UUID
	basePath                          = "/apis/api.ucp.dev/v1alpha3"
)

func initializeTest(t *testing.T) (*httptest.Server, Client, *aws.MockAWSClient, *aws.MockAWSCloudFormationClient) {
	ctrl := gomock.NewController(t)
	cloudcontrolClient := aws.NewMockAWSClient(ctrl)
	cloudformationClient := aws.NewMockAWSCloudFormationClient(ctrl)

	router := mux.NewRouter()
	ucp := httptest.NewServer(router)
	ctx := context.Background()
	err := api.Register(ctx, router, controller.Options{
		BasePath:                basePath,
		AWSClient:               cloudcontrolClient,
		AWSCloudFormationClient: cloudformationClient,
	})
	require.NoError(t, err)

	ucpClient := NewClient(http.DefaultClient, ucp.URL+basePath)

	return ucp, ucpClient, cloudcontrolClient, cloudformationClient
}
