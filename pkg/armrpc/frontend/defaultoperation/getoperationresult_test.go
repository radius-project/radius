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

package defaultoperation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestGetOperationResultRun(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	rawDataModel := testutil.ReadFixture("operationstatus_datamodel.json")
	osDataModel := &manager.Status{}
	_ = json.Unmarshal(rawDataModel, osDataModel)

	rawExpectedOutput := testutil.ReadFixture("operationstatus_output.json")
	expectedOutput := &v1.AsyncOperationStatus{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	t.Run("get non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, operationStatusTestHeaderFile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		ctl, err := NewGetOperationResult(ctrl.Options{
			StorageClient: mStorageClient,
		})

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	})

	opResTestCases := []struct {
		desc              string
		provisioningState v1.ProvisioningState
		respCode          int
		headersCheck      bool
	}{
		{
			"not-in-terminal-state",
			v1.ProvisioningStateAccepted,
			http.StatusAccepted,
			true,
		},
		{
			"put-succeeded-state",
			v1.ProvisioningStateSucceeded,
			http.StatusNoContent,
			false,
		},
		{
			"delete-succeeded-state",
			v1.ProvisioningStateSucceeded,
			http.StatusNoContent,
			false,
		},
		{
			"put-failed-state",
			v1.ProvisioningStateFailed,
			http.StatusNoContent,
			false,
		},
		{
			"delete-failed-state",
			v1.ProvisioningStateFailed,
			http.StatusNoContent,
			false,
		},
	}

	for _, tt := range opResTestCases {
		t.Run(tt.desc, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, operationStatusTestHeaderFile, nil)
			ctx := testutil.ARMTestContextFromRequest(req)

			osDataModel.Status = tt.provisioningState
			osDataModel.RetryAfter = time.Second * 5

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id},
						Data:     osDataModel,
					}, nil
				})

			ctl, err := NewGetOperationResult(ctrl.Options{
				StorageClient: mStorageClient,
			})

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.respCode, w.Result().StatusCode)

			if tt.headersCheck {
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, req.URL.String(), w.Header().Get("Location"))

				require.NotNil(t, w.Header().Get("Retry-After"))
				require.Equal(t, "5", w.Header().Get("Retry-After"))
			}
		})
	}
}
