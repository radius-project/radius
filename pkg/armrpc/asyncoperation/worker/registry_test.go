// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/stretchr/testify/require"
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

	ctrl := registry.Get(opGet)
	require.NotNil(t, ctrl)
	ctrl = registry.Get(opPut)
	require.NotNil(t, ctrl)
}
