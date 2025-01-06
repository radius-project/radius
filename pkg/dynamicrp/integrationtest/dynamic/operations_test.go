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

package dynamic

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/testhost"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// This test covers the basic functionality of the operation status/result controllers.
//
// This test is synthetic because we don't have a real operation to test against.
func Test_Dynamic_OperationResultAndStatus(t *testing.T) {
	ctx := testcontext.New(t)
	dynamic, ucp := testhost.Start(t)

	// Setup a plane & resource provider & location
	plane := createRadiusPlane(ucp)
	createResourceProvider(ucp)
	createLocation(ucp, "testResources")

	// Now we can make a request to the operation result/status endpoints.
	operationName := uuid.New().String()

	operationResultID := fmt.Sprintf("/planes/radius/%s/providers/%s/locations/global/operationResults/%s", *plane.Name, resourceProviderNamespace, operationName)
	operationStatusID := fmt.Sprintf("/planes/radius/%s/providers/%s/locations/global/operationStatuses/%s", *plane.Name, resourceProviderNamespace, operationName)

	// This operation doesn't exist yet, so we should get a 404.
	response := ucp.MakeRequest("GET", fmt.Sprintf("%s?api-version=%s", operationResultID, apiVersion), nil)
	response.EqualsErrorCode(http.StatusNotFound, "NotFound")

	response = ucp.MakeRequest("GET", fmt.Sprintf("%s?api-version=%s", operationStatusID, apiVersion), nil)
	response.EqualsErrorCode(http.StatusNotFound, "NotFound")

	// Now let's simulate the creation of an operation, by putting one in the database.
	databaseClient, err := dynamic.Options().DatabaseProvider.GetClient(ctx)
	require.NoError(t, err)

	operation := &statusmanager.Status{
		AsyncOperationStatus: v1.AsyncOperationStatus{
			ID:     operationStatusID,
			Name:   operationName,
			Status: v1.ProvisioningStateUpdating,
		},
	}

	err = databaseClient.Save(ctx, &database.Object{Data: operation, Metadata: database.Metadata{ID: operationStatusID}})
	require.NoError(t, err)

	// Now let's query it again, we should find it.
	response = ucp.MakeRequest("GET", fmt.Sprintf("%s?api-version=%s", operationResultID, apiVersion), nil)
	response.EqualsStatusCode(http.StatusAccepted)

	response = ucp.MakeRequest("GET", fmt.Sprintf("%s?api-version=%s", operationStatusID, apiVersion), nil)
	response.EqualsStatusCode(http.StatusOK)
}
