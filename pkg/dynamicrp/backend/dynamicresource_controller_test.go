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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/stretchr/testify/require"
)

func Test_DynamicResourceController_selectController(t *testing.T) {
	setup := func() *DynamicResourceController {
		opts := ctrl.Options{}
		controller, err := NewDynamicResourceController(opts)
		require.NoError(t, err)
		return controller.(*DynamicResourceController)
	}

	t.Run("inert PUT", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/test-resource",
			OperationType: v1.OperationType{Type: "Applications.Test/testResources", Method: v1.OperationPut}.String(),
		}

		selected, err := controller.selectController(request)
		require.NoError(t, err)

		require.IsType(t, &InertPutController{}, selected)
	})

	t.Run("inert DELETE", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/test-resource",
			OperationType: v1.OperationType{Type: "Applications.Test/testResources", Method: v1.OperationDelete}.String(),
		}

		selected, err := controller.selectController(request)
		require.NoError(t, err)

		require.IsType(t, &InertDeleteController{}, selected)
	})

	t.Run("unknown operation", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/test-resource",
			OperationType: v1.OperationType{Type: "Applications.Test/testResources", Method: v1.OperationGet}.String(),
		}

		selected, err := controller.selectController(request)
		require.Error(t, err)
		require.Equal(t, "unsupported operation type: \"APPLICATIONS.TEST/TESTRESOURCES|GET\"", err.Error())
		require.Nil(t, selected)
	})
}
