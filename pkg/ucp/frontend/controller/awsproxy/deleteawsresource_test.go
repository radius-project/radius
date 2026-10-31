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
package awsproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The tests in this file cover a DELETE of a URL that has no record, which is transitional: UCP deletes it in AWS by
// the URL's last segment, as it did before records. Deletes of records are covered in engine_test.go.

// ownerHeaders are the owner headers a DELETE with no record accepts: none, as Deployment Engine releases before
// azure-octo/deployment-engine#705 send, or an owner, as recipe deletes send.
var ownerHeaders = []struct {
	name  string
	owner string
}{
	{name: "without the owner header"},
	{name: "with the owner header", owner: testOwner},
}

func runDeleteWithoutRecord(t *testing.T, testOptions TestOptions, path string, owner string) *http.Response {
	t.Helper()
	awsController, err := NewDeleteAWSResource(testOptions.ControllerOptions(), testOptions.Clients())
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodDelete, path, nil)
	require.NoError(t, err)
	if owner != "" {
		request.Header.Set(sdkclients.AWSResourceOwnerHeader, owner)
	}

	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := awsController.Run(ctx, nil, request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	require.NoError(t, actualResponse.Apply(ctx, w, request))
	return w.Result()
}

func Test_DeleteAWSResource(t *testing.T) {
	for _, tt := range ownerHeaders {
		t.Run(tt.name, func(t *testing.T) {
			testResource := CreateKinesisStreamTestResource(uuid.New().String())
			testOptions := setupTest(t)
			testOptions.AWSCloudControlClient.EXPECT().DeleteResource(gomock.Any(), &cloudcontrol.DeleteResourceInput{
				TypeName:   aws.String(testResource.AWSResourceType),
				Identifier: aws.String(testResource.ResourceName),
			}, gomock.Any()).Return(
				&cloudcontrol.DeleteResourceOutput{
					ProgressEvent: &types.ProgressEvent{
						OperationStatus: types.OperationStatusSuccess,
						RequestToken:    aws.String(testAWSRequestToken),
					},
				}, nil)

			res := runDeleteWithoutRecord(t, testOptions, testResource.SingleResourcePath, tt.owner)
			defer res.Body.Close()
			require.Equal(t, http.StatusAccepted, res.StatusCode)
			require.Contains(t, res.Header.Get("Azure-AsyncOperation"), "/operationStatuses/"+uuid.MustParse(testAWSRequestToken).String())
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, []byte("{}"), body)
		})
	}
}

func Test_DeleteAWSResource_ResourceDoesNotExist(t *testing.T) {
	for _, tt := range ownerHeaders {
		t.Run(tt.name, func(t *testing.T) {
			testResource := CreateKinesisStreamTestResource(uuid.New().String())
			testOptions := setupTest(t)
			testOptions.AWSCloudControlClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				nil, &types.ResourceNotFoundException{
					Message: aws.String("Resource not found"),
				})

			res := runDeleteWithoutRecord(t, testOptions, testResource.SingleResourcePath, tt.owner)
			defer res.Body.Close()
			require.Equal(t, http.StatusNoContent, res.StatusCode)
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, []byte(""), body)
		})
	}
}

// Test_DeleteAWSResource_BlankOwner checks that an owner header that is sent is still validated.
func Test_DeleteAWSResource_BlankOwner(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.New().String())
	res := runDeleteWithoutRecord(t, setupTest(t), testResource.SingleResourcePath, "  ")
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func Test_DeleteAWSResource_AWSError(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.New().String())
	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, awsError(&smithy.GenericAPIError{Code: "AccessDenied", Message: "no", Fault: smithy.FaultClient}))

	res := runDeleteWithoutRecord(t, testOptions, testResource.SingleResourcePath, testOwner)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func Test_DeleteAWSResource_InvalidRequestToken(t *testing.T) {
	testResource := CreateKinesisStreamTestResource(uuid.New().String())
	testOptions := setupTest(t)
	testOptions.AWSCloudControlClient.EXPECT().DeleteResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.DeleteResourceOutput{
			ProgressEvent: &types.ProgressEvent{RequestToken: aws.String("not-a-uuid")},
		}, nil)

	awsController, err := NewDeleteAWSResource(testOptions.ControllerOptions(), testOptions.Clients())
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodDelete, testResource.SingleResourcePath, nil)
	require.NoError(t, err)
	_, err = awsController.Run(rpctest.NewARMRequestContext(request), nil, request)
	require.Error(t, err)
}
