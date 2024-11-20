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

package worker

import (
	"context"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/corerp/backend/deployment"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRegister_Get(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	registry := NewControllerRegistry(mockSP)

	opGet := v1.OperationType{Type: "Applications.Core/environments", Method: v1.OperationGet}
	opPut := v1.OperationType{Type: "Applications.Core/environments", Method: v1.OperationPut}

	ctrlOpts := ctrl.Options{
		StorageClient:          nil,
		DataProvider:           mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor { return nil },
	}

	err := registry.Register(context.TODO(), opGet.Type, opGet.Method, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &testAsyncController{
			BaseController: ctrl.NewBaseAsyncController(ctrlOpts),
			fn: func(ctx context.Context) (ctrl.Result, error) {
				return ctrl.Result{}, nil
			},
		}, nil
	}, ctrlOpts)
	require.NoError(t, err)

	err = registry.Register(context.TODO(), opPut.Type, opPut.Method, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &testAsyncController{
			BaseController: ctrl.NewBaseAsyncController(ctrlOpts),
		}, nil
	}, ctrlOpts)
	require.NoError(t, err)

	ctrl, err := registry.Get(context.Background(), opGet)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	ctrl, err = registry.Get(context.Background(), opPut)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Getting a controller that is not registered should return nil by default.
	ctrl, err = registry.Get(context.Background(), v1.OperationType{Type: "Applications.Core/unknown", Method: v1.OperationGet})
	require.NoError(t, err)
	require.Nil(t, ctrl)
}

func TestRegister_Get_WithDefault(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	registry := NewControllerRegistry(mockSP)

	opGet := v1.OperationType{Type: "Applications.Core/environments", Method: v1.OperationGet}

	ctrlOpts := ctrl.Options{
		StorageClient:          nil,
		DataProvider:           mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor { return nil },
	}

	err := registry.Register(context.TODO(), opGet.Type, opGet.Method, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &testAsyncController{
			BaseController: ctrl.NewBaseAsyncController(ctrlOpts),
			fn: func(ctx context.Context) (ctrl.Result, error) {
				return ctrl.Result{}, nil
			},
		}, nil
	}, ctrlOpts)
	require.NoError(t, err)

	err = registry.RegisterDefault(context.TODO(), func(opts ctrl.Options) (ctrl.Controller, error) {
		return &testAsyncController{
			BaseController: ctrl.NewBaseAsyncController(ctrlOpts),
		}, nil
	}, ctrlOpts)
	require.NoError(t, err)

	ctrl, err := registry.Get(context.Background(), opGet)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Getting a controller that is not registered should default the default
	ctrl, err = registry.Get(context.Background(), v1.OperationType{Type: "Applications.Core/unknown", Method: v1.OperationGet})
	require.NoError(t, err)
	require.NotNil(t, ctrl)
}
