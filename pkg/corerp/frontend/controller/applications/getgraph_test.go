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

package applications

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestGetGraphRun_20231001Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/Applications/myapp/getGraph?api-version=2023-10-01-preview", nil)

	require.NoError(t, err)

	t.Run("resource not found", func(t *testing.T) {
		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(nil, &store.ErrNotFound{})
		ctx := rpctest.NewARMRequestContext(req)
		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		conn, _ := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")

		ctl, err := NewGetGraph(opts, conn)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
		require.NoError(t, err)
	})
}
