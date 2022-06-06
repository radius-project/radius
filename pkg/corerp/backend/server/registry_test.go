// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestRegister_Get(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	registry := NewControllerRegistry(mockSP)

	opGet := asyncoperation.OperationType{Type: "Applications.Core/environments", Method: asyncoperation.OperationGet}
	opPut := asyncoperation.OperationType{Type: "Applications.Core/environments", Method: asyncoperation.OperationPut}

	err := registry.Register(context.TODO(), opGet, func(s store.StorageClient) (asyncoperation.Controller, error) {
		return &testAsyncController{
			BaseController: asyncoperation.NewBaseAsyncController(nil),
			fn: func(ctx context.Context) (asyncoperation.Result, error) {
				return asyncoperation.Result{}, nil
			},
		}, nil
	})
	require.NoError(t, err)

	err = registry.Register(context.TODO(), opPut, func(s store.StorageClient) (asyncoperation.Controller, error) {
		return &testAsyncController{
			BaseController: asyncoperation.NewBaseAsyncController(nil),
		}, nil
	})
	require.NoError(t, err)

	ctrl := registry.Get(opGet)
	require.NotNil(t, ctrl)
	ctrl = registry.Get(opPut)
	require.NotNil(t, ctrl)
}
