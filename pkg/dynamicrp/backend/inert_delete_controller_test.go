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

package backend

import (
	"testing"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_InertDeleteController_Run(t *testing.T) {
	setup := func() (*InertDeleteController, *database.MockClient) {
		mockctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(mockctrl)

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		controller, err := NewInertDeleteController(opts)
		require.NoError(t, err)
		return controller.(*InertDeleteController), databaseClient
	}

	controller, databaseClient := setup()

	request := &ctrl.Request{
		ResourceID: "/planes/radius/testing/resourceGroups/test-group/providers/Applications.Test/exampleResources/my-example",
	}

	// Controller needs to call delete on the resource.
	databaseClient.EXPECT().Delete(gomock.Any(), request.ResourceID).Return(nil).Times(1)

	result, err := controller.Run(testcontext.New(t), request)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}
