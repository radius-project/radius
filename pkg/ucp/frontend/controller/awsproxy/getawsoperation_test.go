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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"uuid"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const testOperationResourceID = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Kinesis/Stream/stream"

// queueTestOperation queues an operation on testOperationResourceID and returns its ID.
func queueTestOperation(t *testing.T, options TestOptions) uuid.UUID {
	t.Helper()
	id, err := resources.ParseResource(testOperationResourceID)
	require.NoError(t, err)
	operationID := uuid.New()
	err = options.StatusManager.QueueAsyncOperation(t.Context(), &v1.ARMRequestContext{
		ResourceID:    id,
		OperationID:   operationID,
		OperationType: v1.OperationType{Type: OperationTypeAWSResourceForTest, Method: v1.OperationPut},
	}, statusmanagerOptions())
	require.NoError(t, err)
	return operationID
}

func runOperationRequest(t *testing.T, factory func(armrpc_controller.Options) (armrpc_controller.Controller, error), options TestOptions, path string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	controller, err := factory(options.ControllerOptions())
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(request)
	response, err := controller.Run(ctx, nil, request)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.NoError(t, response.Apply(ctx, w, request))
	return w.Result()
}

func operationPath(kind string, operationID string) string {
	return "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Kinesis/locations/global/" + kind + "/" + operationID
}

func Test_GetAWSOperationStatuses(t *testing.T) {
	options := setupTest(t)
	operationID := queueTestOperation(t, options)

	res := runOperationRequest(t, NewGetAWSOperationStatuses, options, operationPath("operationStatuses", operationID.String()))
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	status := v1.AsyncOperationStatus{}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&status))
	require.Equal(t, v1.ProvisioningStateAccepted, status.Status)

	id, err := resources.ParseResource(testOperationResourceID)
	require.NoError(t, err)
	require.NoError(t, options.StatusManager.Update(t.Context(), id, operationID, v1.ProvisioningStateFailed, nil, &v1.ErrorDetails{Code: v1.CodeInvalid, Message: "AWS rejected it"}))

	res = runOperationRequest(t, NewGetAWSOperationStatuses, options, operationPath("operationStatuses", operationID.String()))
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&status))
	require.Equal(t, v1.ProvisioningStateFailed, status.Status)
	require.Equal(t, "AWS rejected it", status.Error.Message)
}

func Test_GetAWSOperationResults(t *testing.T) {
	options := setupTest(t)
	operationID := queueTestOperation(t, options)

	res := runOperationRequest(t, NewGetAWSOperationResults, options, operationPath("operationResults", operationID.String()))
	defer res.Body.Close()
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	require.NotEmpty(t, res.Header.Get("Location"))

	id, err := resources.ParseResource(testOperationResourceID)
	require.NoError(t, err)
	require.NoError(t, options.StatusManager.Update(t.Context(), id, operationID, v1.ProvisioningStateSucceeded, nil, nil))

	res = runOperationRequest(t, NewGetAWSOperationResults, options, operationPath("operationResults", operationID.String()))
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func Test_GetAWSOperation_NotFound(t *testing.T) {
	for _, operationID := range []string{uuid.New().String(), "not-a-uuid"} {
		for kind, factory := range map[string]func(armrpc_controller.Options) (armrpc_controller.Controller, error){
			"operationStatuses": NewGetAWSOperationStatuses,
			"operationResults":  NewGetAWSOperationResults,
		} {
			t.Run(kind+" "+operationID, func(t *testing.T) {
				res := runOperationRequest(t, factory, setupTest(t), operationPath(kind, operationID))
				defer res.Body.Close()
				require.Equal(t, http.StatusNotFound, res.StatusCode)
			})
		}
	}
}

func Test_GetAWSOperation_InvalidRegion(t *testing.T) {
	path := "/planes/aws/aws/accounts/1234567/providers/AWS.Kinesis/locations/global/operationStatuses/" + uuid.New().String()
	res := runOperationRequest(t, NewGetAWSOperationStatuses, setupTest(t), path)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	res = runOperationRequest(t, NewGetAWSOperationResults, setupTest(t), path)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func Test_GetAWSOperation_NoStatusManager(t *testing.T) {
	options := setupTest(t)
	options.StatusManager = nil
	request, err := http.NewRequest(http.MethodGet, operationPath("operationStatuses", uuid.New().String()), nil)
	require.NoError(t, err)
	controller, err := NewGetAWSOperationStatuses(options.ControllerOptions())
	require.NoError(t, err)
	_, err = controller.Run(rpctest.NewARMRequestContext(request), nil, request)
	require.ErrorContains(t, err, "status manager is not configured")
}
